package keystore

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testKey(b byte) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Repeat(string(rune(b)), keySize)))
}

// fileEnv chooses the file store in a fresh directory, for the whole test.
func fileEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(EnvKind, "")
	t.Setenv(EnvDir, dir)
	t.Setenv(EnvKey, testKey('k'))
	t.Setenv(ProfileEnv("petctl"), "")
	return dir
}

func TestTheFileStoreKeepsASecretEncrypted(t *testing.T) {
	dir := fileEnv(t)
	store := Keyring("petctl", "token")
	if _, err := store.Get(); err != ErrNotFound {
		t.Fatalf("before a login: %v", err)
	}
	long := strings.Repeat("refresh-token-", 400)
	if err := store.Set(long); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(); err != nil || got != long {
		t.Fatalf("get = %d bytes, %v", len(got), err)
	}

	path := filepath.Join(dir, "petctl", "token")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "refresh-token") {
		t.Error("the secret is in the file in the clear")
	}
	for p, want := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: mode %v, want %v (%v)", p, info.Mode().Perm(), want, err)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("files left beside the secret: %v", entries)
	}

	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(); err != ErrNotFound {
		t.Errorf("after delete: %v", err)
	}
	if err := store.Delete(); err != nil {
		t.Errorf("a second delete: %v", err)
	}
}

func TestAnotherKeyNeitherOpensNorReplacesTheFile(t *testing.T) {
	fileEnv(t)
	store := Keyring("petctl", "token")
	if err := store.Set("the good login"); err != nil {
		t.Fatal(err)
	}

	t.Setenv(EnvKey, testKey('x'))
	if _, err := store.Get(); err == nil || !strings.Contains(err.Error(), EnvKey) {
		t.Errorf("get with another key: %v", err)
	}
	if err := store.Set("a login over it"); err == nil || !strings.Contains(err.Error(), "Nothing was replaced") {
		t.Errorf("set with another key: %v", err)
	}

	t.Setenv(EnvKey, testKey('k'))
	if got, err := store.Get(); err != nil || got != "the good login" {
		t.Errorf("with the right key again: %q, %v", got, err)
	}
}

func TestASealedFileOnlyOpensUnderItsOwnName(t *testing.T) {
	dir := fileEnv(t)
	if err := Keyring("petctl", "token").Set("secret"); err != nil {
		t.Fatal(err)
	}
	from := filepath.Join(dir, "petctl", "token")
	for _, to := range []string{filepath.Join(dir, "petctl", "pending"), filepath.Join(dir, "dogctl", "token")} {
		data, _ := os.ReadFile(from)
		_ = os.MkdirAll(filepath.Dir(to), 0o700)
		if err := os.WriteFile(to, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Keyring("petctl", "pending").Get(); err == nil {
		t.Error("the file opened under another name")
	}
	if _, err := Keyring("dogctl", "token").Get(); err == nil {
		t.Error("the file opened for another program")
	}
}

func TestADamagedFileIsAnError(t *testing.T) {
	dir := fileEnv(t)
	store := Keyring("petctl", "token")
	if err := store.Set("secret"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "petctl", "token")
	data, _ := os.ReadFile(path)
	data[len(data)-1] ^= 1
	_ = os.WriteFile(path, data, 0o600)
	if _, err := store.Get(); err == nil {
		t.Error("a changed file opened")
	}
	_ = os.WriteFile(path, []byte("plain"), 0o600)
	if _, err := store.Get(); err == nil || !strings.Contains(err.Error(), "not written by ctlkit") {
		t.Errorf("a file that is not sealed: %v", err)
	}
}

func TestProfilesKeepLoginsApart(t *testing.T) {
	dir := fileEnv(t)
	if err := Keyring("petctl", "token").Set("kari-login"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ProfileEnv("petctl"), "ola")
	store := Keyring("petctl", "token")
	if _, err := store.Get(); err != ErrNotFound {
		t.Fatalf("a new profile sees another login: %v", err)
	}
	if err := store.Set("ola"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "petctl", "profiles", "ola", "token")); err != nil {
		t.Errorf("the profile's file: %v", err)
	}

	t.Setenv(ProfileEnv("petctl"), "")
	if got, _ := Keyring("petctl", "token").Get(); got != "kari-login" {
		t.Errorf("the default profile: %q", got)
	}
	t.Setenv(ProfileEnv("petctl"), "default")
	if got, _ := Keyring("petctl", "token").Get(); got != "kari-login" {
		t.Errorf(`the profile "default": %q`, got)
	}

	t.Setenv(ProfileEnv("petctl"), "../x")
	if err := Keyring("petctl", "token").Set("x"); err == nil {
		t.Error("a profile that is a path was used")
	}
}

func TestASharedSecretIsTheSameInEveryProfile(t *testing.T) {
	fileEnv(t)
	if err := Shared("petctl", "api-key").Set("key"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ProfileEnv("petctl"), "ola")
	if got, err := Shared("petctl", "api-key").Get(); err != nil || got != "key" {
		t.Errorf("in another profile: %q, %v", got, err)
	}
	if _, err := Keyring("petctl", "api-key").Get(); err != ErrNotFound {
		t.Errorf("a profile's own store sees the shared secret: %v", err)
	}
}

func TestTheDefaultProfileKeepsTheOldNames(t *testing.T) {
	if got := Profile(DefaultProfile).entry("token"); got != "token" {
		t.Errorf("default: %q", got)
	}
	if got := Profile("ola").entry("token"); got != "profiles/ola/token" {
		t.Errorf("ola: %q", got)
	}
}

func TestConfigFrom(t *testing.T) {
	key := testKey('k')
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	cases := []struct {
		name    string
		getenv  func(string) string
		kind    Kind
		dir     string
		failure string
	}{
		{"nothing set is the keyring", env(), SystemKeyring, "", ""},
		{"keyring by name", env(EnvKind, "keyring", EnvDir, "/x"), SystemKeyring, "", ""},
		{"a directory chooses files", env(EnvDir, "/data/keys", EnvKey, key), File, "/data/keys/petctl", ""},
		{"files under XDG_STATE_HOME", env(EnvKind, "file", "XDG_STATE_HOME", "/state", EnvKey, key), File, "/state/petctl", ""},
		{"files need a key", env(EnvKind, "file", "XDG_STATE_HOME", "/state"), "", "", "openssl rand -base64 32"},
		{"a short key", env(EnvDir, "/d", EnvKey, base64.StdEncoding.EncodeToString([]byte("short"))), "", "", "not 32 bytes"},
		{"an unknown kind", env(EnvKind, "vault"), "", "", `use "keyring" or "file"`},
		{"a relative directory", env(EnvDir, "keys", EnvKey, key), "", "", "absolute"},
	}
	for _, c := range cases {
		got, err := ConfigFrom(c.getenv, "petctl")
		switch {
		case c.failure != "" && (err == nil || !strings.Contains(err.Error(), c.failure)):
			t.Errorf("%s: err = %v, want %q", c.name, err, c.failure)
		case c.failure == "" && (err != nil || got.Kind != c.kind || got.Dir != c.dir):
			t.Errorf("%s: %+v, %v", c.name, got, err)
		}
	}
}

func TestDescribeNamesNoSecret(t *testing.T) {
	dir := fileEnv(t)
	got := Describe("petctl")
	if !strings.Contains(got, filepath.Join(dir, "petctl")) || strings.Contains(got, testKey('k')) {
		t.Errorf("describe = %q", got)
	}
	if LockDir("petctl") != filepath.Join(dir, "petctl") {
		t.Errorf("lock dir = %q", LockDir("petctl"))
	}
}
