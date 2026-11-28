package session

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/langtind/ctlkit/keystore"
)

// helperRenew is one run of a tool in its own process, with the file store.
// The service is a file: the refresh token it takes, and one line per
// renewal. A renewal with a token that was used before fails, as Coop's does.
func helperRenew() {
	service := os.Getenv("CTLKIT_TEST_SERVICE")
	m := &Manager{Store: keystore.Keyring("petctl", "token"), LockPath: LockFile("petctl")}
	m.Renew = func(ctx context.Context, refresh string) (Session, error) {
		valid, _ := os.ReadFile(filepath.Join(service, "valid"))
		if string(valid) != refresh {
			return Session{}, fmt.Errorf("the refresh token %s was used before", refresh)
		}
		time.Sleep(200 * time.Millisecond)
		log, _ := os.OpenFile(filepath.Join(service, "renewals"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		_, _ = log.WriteString(refresh + "\n")
		_ = log.Close()
		next := refresh + "+"
		_ = os.WriteFile(filepath.Join(service, "valid"), []byte(next), 0o600)
		return Session{AccessToken: "access-" + next, AccessExpires: time.Now().Add(time.Hour), RefreshToken: next}, nil
	}
	if _, err := m.Token(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestMain(m *testing.M) {
	if os.Getenv("CTLKIT_TEST_HELPER") == "renew" {
		helperRenew()
	}
	os.Exit(m.Run())
}

func TestRunsInSeparateProcessesWithTheFileStoreRenewOnce(t *testing.T) {
	store, service := t.TempDir(), t.TempDir()
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	t.Setenv(keystore.EnvDir, store)
	t.Setenv(keystore.EnvKey, key)
	t.Setenv("PETCTL_PROFILE", "")

	m := &Manager{Store: keystore.Keyring("petctl", "token")}
	if err := m.Save(Session{AccessToken: "old", AccessExpires: time.Now().Add(-time.Minute), RefreshToken: "r0"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(service, "valid"), []byte("r0"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LockFile("petctl"); got != filepath.Join(store, "petctl", "renew.lock") {
		t.Fatalf("lock file = %s, want it in the store", got)
	}

	runs := make([]*exec.Cmd, 6)
	for i := range runs {
		runs[i] = exec.Command(os.Args[0], "-test.run=^$")
		runs[i].Env = append(os.Environ(), "CTLKIT_TEST_HELPER=renew", "CTLKIT_TEST_SERVICE="+service)
		if err := runs[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for i, run := range runs {
		if err := run.Wait(); err != nil {
			t.Errorf("run %d: %v", i, err)
		}
	}
	log, _ := os.ReadFile(filepath.Join(service, "renewals"))
	if got := strings.Fields(string(log)); len(got) != 1 {
		t.Errorf("renewals = %v, want one", got)
	}
	s, err := m.Load()
	if err != nil || s.RefreshToken != "r0+" {
		t.Errorf("stored = %+v, %v", s, err)
	}
}
