package ctltest

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/langtind/ctlkit"
	"github.com/langtind/ctlkit/session"
)

// Call is one request a tool sent to a Fake.
type Call struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	// Token is the bearer token, without "Bearer ".
	Token string
	// Cookies holds the cookies by name.
	Cookies map[string]string
	// Body is the body as it was sent. For a multipart form it is empty,
	// and Form and Files hold the parts.
	Body  string
	Form  map[string]string
	Files []File
}

// File is one file in a multipart form a tool sent.
type File struct {
	Field       string
	Name        string
	ContentType string
	Data        string
}

// Answer is what a Fake says to one request.
type Answer struct {
	Status int
	Body   string
	// Header is added to the answer. Set-Cookie goes here.
	Header http.Header
}

// Fake is a service that does not exist. It answers what a test tells it
// to and keeps every request it got. A fake answers whatever the test
// wrote, so green tests say nothing about the real service.
type Fake struct {
	t      *testing.T
	server *httptest.Server
	routes map[string]func(Call) Answer

	// Calls is every request since the last Reset, in order.
	Calls []Call
	// Check, when set, sees every request before it is answered. Use it
	// for what every request must carry, such as the user agent.
	Check func(t *testing.T, c Call)
}

// NewFake starts a fake service that lives as long as the test.
func NewFake(t *testing.T) *Fake {
	t.Helper()
	f := &Fake{t: t, routes: map[string]func(Call) Answer{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

// URL is the base address of the fake, and Client an HTTP client for it.
func (f *Fake) URL() string          { return f.server.URL }
func (f *Fake) Client() *http.Client { return f.server.Client() }

// On answers a request with a status and a body. route is the method and
// the path: "GET /things/t1". A later On for the same route replaces it.
func (f *Fake) On(route string, status int, body string) {
	f.Handle(route, func(Call) Answer { return Answer{Status: status, Body: body} })
}

// Handle answers a request with whatever answer returns, for an answer that
// depends on the request or on how many came before it.
func (f *Fake) Handle(route string, answer func(Call) Answer) { f.routes[route] = answer }

// Reset forgets the requests, and keeps the routes.
func (f *Fake) Reset() { f.Calls = nil }

// Sent lists the requests as "METHOD path", separated by commas.
func (f *Fake) Sent() string {
	parts := make([]string, len(f.Calls))
	for i, c := range f.Calls {
		parts[i] = c.Method + " " + c.Path
	}
	return strings.Join(parts, ", ")
}

// Changes lists the requests that were not a GET or a HEAD, on any route
// but those named in except. A login or a renewal is named there.
func (f *Fake) Changes(except ...string) []string {
	var changes []string
	for _, c := range f.Calls {
		route := c.Method + " " + c.Path
		if c.Method == http.MethodGet || c.Method == http.MethodHead || containsString(except, route) {
			continue
		}
		changes = append(changes, route)
	}
	return changes
}

func containsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	c := Call{
		Method:  r.Method,
		Path:    r.URL.Path,
		Query:   r.URL.RawQuery,
		Header:  r.Header.Clone(),
		Token:   strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
		Cookies: map[string]string{},
	}
	for _, cookie := range r.Cookies() {
		c.Cookies[cookie.Name] = cookie.Value
	}
	if kind, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); kind == "multipart/form-data" {
		c.Form = map[string]string{}
		reader := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(part)
			if part.FileName() == "" {
				c.Form[part.FormName()] = string(data)
				continue
			}
			c.Files = append(c.Files, File{Field: part.FormName(), Name: part.FileName(), ContentType: part.Header.Get("Content-Type"), Data: string(data)})
		}
	} else {
		body, _ := io.ReadAll(r.Body)
		c.Body = string(body)
	}
	f.Calls = append(f.Calls, c)
	if f.Check != nil {
		f.Check(f.t, c)
	}

	answer := Answer{Status: http.StatusNotFound, Body: `{"message":"the fake has no answer for ` + c.Method + ` ` + c.Path + `"}`}
	if route, ok := f.routes[c.Method+" "+c.Path]; ok {
		answer = route(c)
	}
	for name, values := range answer.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	if answer.Status == 0 {
		answer.Status = http.StatusOK
	}
	w.WriteHeader(answer.Status)
	fmt.Fprint(w, answer.Body)
}

// ChangeNeedsAYes checks the two promises of a command that changes
// something: with --dry-run it sends no change, and without --yes and
// without a terminal it refuses and sends none. args is the command with
// what it needs, and without --dry-run and --yes. except names routes that
// are not the change, such as "POST /login/refresh".
func ChangeNeedsAYes(t *testing.T, build func() *ctlkit.Program, fake *Fake, args []string, except ...string) {
	t.Helper()
	fake.Reset()
	code, stdout, stderr := Run(build(), "", append(append([]string{}, args...), "--dry-run")...)
	if code != 0 || len(fake.Changes(except...)) != 0 {
		t.Errorf("%v --dry-run: exit = %d, changes sent = %v\nstdout: %s\nstderr: %s", args, code, fake.Changes(except...), stdout, stderr)
	}

	fake.Reset()
	code, _, stderr = Run(build(), "", args...)
	if code != 2 || !strings.Contains(stderr, "refusing without confirmation") || len(fake.Changes(except...)) != 0 {
		t.Errorf("%v without --yes: exit = %d, changes sent = %v, stderr = %q", args, code, fake.Changes(except...), stderr)
	}
	fake.Reset()
}

// RefreshRenewsEveryTime runs "refresh" twice in a row and fails unless the
// stored access token changed both times. The second run is the one that
// tells: its access token is still good, and a tool that renews only a
// token that has run out does nothing there. The fake has to answer a
// renewal the way the real service does, with a new token each time.
func RefreshRenewsEveryTime(t *testing.T, build func() *ctlkit.Program, stored func() session.Session) {
	t.Helper()
	before := stored().AccessToken
	if before == "" {
		t.Fatal("RefreshRenewsEveryTime: no login is stored")
	}
	for run := 1; run <= 2; run++ {
		code, _, stderr := Run(build(), "", "refresh")
		after := stored().AccessToken
		if code != 0 || after == before || after == "" {
			t.Fatalf("refresh, run %d: exit = %d, the access token changed = %t, stderr = %q", run, code, after != before && after != "", stderr)
		}
		before = after
	}
}
