// Package scripts_test runs the discovery scripts against a service that
// does not exist. They need bash, curl and jq.
package scripts_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, stdin, script string, args ...string) string {
	t.Helper()
	for _, tool := range []string{"bash", "curl", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	path, err := filepath.Abs(script)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", script, args, err, out)
	}
	return string(out)
}

func TestJSONShapeShowsNoValue(t *testing.T) {
	answer := `{"name":"Kari Nordmann","id":4711,"sent":"2026-10-15T08:00:00","read":null,
		"children":[{"firstName":"Ola"},{"firstName":"Per"}],"none":[],"ok":true}`
	got := strings.Join(strings.Fields(run(t, answer, "json-shape")), "")
	want := `{"name":"string(13)","id":"number","sent":"time","read":null,"children":[{"firstName":"string(3)"},"2inall"],"none":[],"ok":true}`
	if got != want {
		t.Errorf("shape = %s", got)
	}
	for _, secret := range []string{"Kari", "Ola", "4711", "2026"} {
		if strings.Contains(got, secret) {
			t.Errorf("the shape holds %q", secret)
		}
	}
}

func TestBundleScriptsFollowChunksAndFindPaths(t *testing.T) {
	files := map[string]string{
		"/app/":                     `<html><script src="main.abc.js"></script><script src="/static/vendor.js"></script></html>`,
		"/app/main.abc.js":          `import("./chunk-AAAA.js");const mime="application/json";`,
		"/app/chunk-AAAA.js":        `e.u=n=>"chunk-BBBB.js";var img="/assets/logo.png"`,
		"/app/chunk-BBBB.js":        "this.http.post({url:`vfs/v1/schools/${s}/applications`,body:f});if(f.reason.length>2000)throw 0;this.http.get({url:\"vfs/v1/schools/causes\"})",
		"/static/vendor.js":         `var lib="left/pad"`,
		"/elsewhere/not-fetched.js": `never`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	dir := filepath.Join(t.TempDir(), "bundle")
	if out := run(t, "", "bundle-fetch", srv.URL+"/app/", dir); !strings.Contains(out, "Fetched 4 script files") {
		t.Fatalf("bundle-fetch: %s", out)
	}
	for _, name := range []string{"app_main.abc.js", "app_chunk-AAAA.js", "app_chunk-BBBB.js", "static_vendor.js"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was not fetched", name)
		}
	}

	paths := run(t, "", "bundle-paths", dir, "vfs")
	if !strings.Contains(paths, "vfs/v1/schools/${s}/applications") || !strings.Contains(paths, "vfs/v1/schools/causes") || strings.Contains(paths, "logo.png") {
		t.Errorf("bundle-paths:\n%s", paths)
	}
	if all := run(t, "", "bundle-paths", dir); strings.Contains(all, "application/json") || strings.Contains(all, ".js") {
		t.Errorf("bundle-paths kept a type or a file:\n%s", all)
	}

	hit := run(t, "", "bundle-grep", dir, "/applications", "40")
	if !strings.Contains(hit, "app_chunk-BBBB.js:") || !strings.Contains(hit, "this.http.post({url:") || strings.Contains(hit, "causes") {
		t.Errorf("bundle-grep:\n%s", hit)
	}
}

func TestCookieMatrixShowsNamesAndNeverValues(t *testing.T) {
	// The fake takes the session id with either token, and renews when it
	// gets the refresh token without the access token.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		has := func(name string) bool { _, err := r.Cookie(name); return err == nil }
		if r.Header.Get("Accept") != "application/json" || !has("sid") || (!has("access") && !has("refresh")) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !has("access") {
			http.SetCookie(w, &http.Cookie{Name: "access", Value: "secret-new-access"})
		}
	}))
	defer srv.Close()

	state := filepath.Join(t.TempDir(), "state.json")
	content := `{"cookies":[
		{"name":"sid","value":"secret-sid","domain":"app.example.invalid"},
		{"name":"access","value":"secret-access","domain":".app.example.invalid"},
		{"name":"refresh","value":"secret-refresh","domain":"app.example.invalid"},
		{"name":"other","value":"secret-other","domain":"login.example.invalid"}]}`
	if err := os.WriteFile(state, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	out := run(t, "", "cookie-matrix", state, "app.example.invalid", srv.URL+"/me", "-H", "Accept: application/json")
	rows := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n")[1:] {
		fields := strings.Fields(line)
		rows[fields[1]] = fields[0] + " " + fields[2]
	}
	want := map[string]string{
		"(none)":             "401 -",
		"access,refresh":     "401 -",
		"access,sid":         "200 -",
		"refresh,sid":        "200 access",
		"access,refresh,sid": "200 -",
	}
	for sent, answer := range want {
		if rows[sent] != answer {
			t.Errorf("%s: got %q, want %q\n%s", sent, rows[sent], answer, out)
		}
	}
	if len(rows) != 8 || strings.Contains(out, "secret") || strings.Contains(out, "other") {
		t.Errorf("rows = %d\n%s", len(rows), out)
	}
}
