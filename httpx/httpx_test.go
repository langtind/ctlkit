package httpx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/langtind/ctlkit"
)

func get(t *testing.T, handler http.HandlerFunc) ([]byte, error) {
	t.Helper()
	srv := httptest.NewServer(handler)
	defer srv.Close()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return Do(srv.Client(), req, "Kennel", "petctl/1.2.3")
}

func TestDoReturnsTheBodyAndSendsTheUserAgent(t *testing.T) {
	body, err := get(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Header.Get("User-Agent"))
	})
	if err != nil || string(body) != "petctl/1.2.3" {
		t.Errorf("body = %q, err = %v", body, err)
	}
}

func TestAStatusBecomesAKind(t *testing.T) {
	tests := []struct {
		status  int
		body    string
		kind    ctlkit.Kind
		message string
	}{
		{401, `{"message":"token expired"}`, ctlkit.KindAuth, "Kennel: HTTP 401: token expired"},
		{403, `{"error":"not yours"}`, ctlkit.KindAuth, "Kennel: HTTP 403: not yours"},
		{404, `<html>gone</html>`, ctlkit.KindNotFound, "Kennel: HTTP 404"},
		{409, `{"reason":"already taken"}`, ctlkit.KindConflict, "Kennel: HTTP 409: already taken"},
		{429, ``, ctlkit.KindRateLimit, "Kennel rate limit reached (HTTP 429), retry after 12 seconds"},
		{503, ``, ctlkit.KindUnavailable, "Kennel: HTTP 503"},
		{400, `{"error":{"code":7}}`, ctlkit.KindFailure, "Kennel: HTTP 400"},
	}
	for _, tt := range tests {
		_, err := get(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "12")
			w.WriteHeader(tt.status)
			fmt.Fprint(w, tt.body)
		})
		if err == nil || ctlkit.KindOf(err) != tt.kind || err.Error() != tt.message || Status(err) != tt.status {
			t.Errorf("HTTP %d: kind = %s, status = %d, err = %v", tt.status, ctlkit.KindOf(err), Status(err), err)
		}
	}
}

func TestNoAnswerIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	_, err := Do(NewClient(), req, "Kennel", "petctl/1.2.3")
	if ctlkit.KindOf(err) != ctlkit.KindUnavailable || Status(err) != 0 {
		t.Errorf("kind = %s, err = %v", ctlkit.KindOf(err), err)
	}
}
