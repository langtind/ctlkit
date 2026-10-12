// Package httpx holds the HTTP plumbing the service clients share. It turns
// an answer outside 2xx into an error with a kind, so that a command does
// not have to.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/langtind/ctlkit"
)

const (
	Timeout = 30 * time.Second

	maxResponseBytes = 32 << 20
)

// NewClient returns an HTTP client with the shared timeout.
func NewClient() *http.Client { return &http.Client{Timeout: Timeout} }

// StatusError is an answer with a status outside 2xx. Do returns it inside
// a *ctlkit.Error, and Status finds it there.
type StatusError struct {
	Service string
	Status  int
	// Message is what the service said, when it said it as JSON.
	Message string
	// RetryAfter is the Retry-After header as the service sent it.
	RetryAfter string
}

func (e *StatusError) Error() string {
	if e.Status == http.StatusTooManyRequests {
		msg := e.Service + " rate limit reached (HTTP 429)"
		if wait := e.wait(); wait != "" {
			msg += ", retry after " + wait
		}
		return msg
	}
	if e.Message != "" {
		return fmt.Sprintf("%s: HTTP %d: %s", e.Service, e.Status, e.Message)
	}
	return fmt.Sprintf("%s: HTTP %d", e.Service, e.Status)
}

func (e *StatusError) wait() string {
	if seconds, err := strconv.Atoi(e.RetryAfter); err == nil {
		return fmt.Sprintf("%d seconds", seconds)
	}
	return e.RetryAfter
}

func (e *StatusError) kind() ctlkit.Kind {
	switch {
	case e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden:
		return ctlkit.KindAuth
	case e.Status == http.StatusNotFound || e.Status == http.StatusGone:
		return ctlkit.KindNotFound
	case e.Status == http.StatusConflict:
		return ctlkit.KindConflict
	case e.Status == http.StatusTooManyRequests:
		return ctlkit.KindRateLimit
	case e.Status == http.StatusRequestTimeout || e.Status >= 500:
		return ctlkit.KindUnavailable
	}
	return ctlkit.KindFailure
}

// Status returns the HTTP status behind err, or 0 when err is not an answer
// from a service.
func Status(err error) int {
	var status *StatusError
	if errors.As(err, &status) {
		return status.Status
	}
	return 0
}

// Do sends req and returns the body of a 2xx answer. Any other status, and
// a request that got no answer, comes back as a *ctlkit.Error with a kind.
// It never retries: a caller that is told to wait reports it.
func Do(client *http.Client, req *http.Request, service, userAgent string) ([]byte, error) {
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		// An interrupted run is not the service's fault, and no retry is due.
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("%s: interrupted", service)
		}
		return nil, ctlkit.Errorf(ctlkit.KindUnavailable, "%s: %w", service, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, ctlkit.Errorf(ctlkit.KindUnavailable, "%s: read response: %w", service, err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("%s: response is larger than %d MB", service, maxResponseBytes>>20)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		status := &StatusError{
			Service:    service,
			Status:     resp.StatusCode,
			Message:    message(body),
			RetryAfter: strings.TrimSpace(resp.Header.Get("Retry-After")),
		}
		return nil, &ctlkit.Error{Kind: status.kind(), Message: status.Error(), RetryAfter: status.wait(), Err: status}
	}
	return body, nil
}

// message reads what a JSON error body says. Anything that is not JSON, such
// as a gateway's HTML page, is left out.
func message(body []byte) string {
	var envelope map[string]any
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	for _, name := range []string{"message", "reason", "error", "errorKey"} {
		if text, ok := envelope[name].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}
