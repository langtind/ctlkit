package ctlkit

import (
	"errors"
	"fmt"
)

// Kind says what went wrong in a way a program can act on. Each kind has its
// own exit code, and with --json it is the "kind" of the error document.
type Kind string

const (
	KindFailure     Kind = "failure"
	KindUsage       Kind = "usage"
	KindNotFound    Kind = "not_found"
	KindAuth        Kind = "auth"
	KindConflict    Kind = "conflict"
	KindRateLimit   Kind = "rate_limit"
	KindUnavailable Kind = "unavailable"
)

// kinds is the one list of error kinds. Exit codes, help and schema read it.
var kinds = []struct {
	kind    Kind
	code    int
	meaning string
}{
	{KindFailure, 1, "the command failed"},
	{KindUsage, 2, "the command was called wrongly, so fix the call before running it again"},
	{KindNotFound, 3, "what was asked for does not exist"},
	{KindAuth, 4, "not logged in, or not allowed"},
	{KindConflict, 5, "the service is already in a state that rules the change out"},
	{KindRateLimit, 6, "the service asked for a pause, so wait the time it names"},
	{KindUnavailable, 7, "the service could not be reached or failed, and a later try may work"},
}

// ExitCode returns the exit code of an error kind.
func ExitCode(kind Kind) int {
	for _, k := range kinds {
		if k.kind == kind {
			return k.code
		}
	}
	return 1
}

// Error is an error with a kind. Any other error is a KindFailure.
type Error struct {
	Kind    Kind
	Message string
	// Hint says what to do next. It is printed on its own line.
	Hint string
	// RetryAfter is how long the service asked the caller to wait, as the
	// service said it.
	RetryAfter string
	Err        error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Err }

// WithHint adds what to do next.
func (e *Error) WithHint(format string, args ...any) *Error {
	e.Hint = fmt.Sprintf(format, args...)
	return e
}

// Errorf makes an error of the given kind. It wraps like fmt.Errorf.
func Errorf(kind Kind, format string, args ...any) *Error {
	err := fmt.Errorf(format, args...)
	return &Error{Kind: kind, Message: err.Error(), Err: errors.Unwrap(err)}
}

// Usagef marks a mistake in how the program was called.
func Usagef(format string, args ...any) *Error {
	return Errorf(KindUsage, format, args...)
}

// NotFoundf says that what was asked for does not exist.
func NotFoundf(format string, args ...any) *Error {
	return Errorf(KindNotFound, format, args...)
}

// Authf says that the caller is not logged in or not allowed.
func Authf(format string, args ...any) *Error {
	return Errorf(KindAuth, format, args...)
}

// KindOf returns the kind of err, and KindFailure for a plain error.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindFailure
}

// classify turns any error into the one that is reported. The message is
// the whole chain, and kind, hint and wait come from the Error inside it.
func (p *Program) classify(err error) *Error {
	out := &Error{Kind: KindFailure, Message: err.Error()}
	var e *Error
	if errors.As(err, &e) {
		out.Kind, out.Hint, out.RetryAfter = e.Kind, e.Hint, e.RetryAfter
	}
	if out.Hint == "" && p.Hint != nil {
		out.Hint = p.Hint(err)
	}
	return out
}
