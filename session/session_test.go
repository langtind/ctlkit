package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/langtind/ctlkit"
	"github.com/langtind/ctlkit/keystore"
)

var now = time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)

// lockedStore is a keystore that several goroutines can use.
type lockedStore struct {
	mu sync.Mutex
	keystore.Memory
}

func (s *lockedStore) Get() (string, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.Memory.Get() }
func (s *lockedStore) Set(v string) error   { s.mu.Lock(); defer s.mu.Unlock(); return s.Memory.Set(v) }

// manager returns a manager whose service hands out a new pair of tokens on
// every renewal and takes each refresh token only once.
func manager(t *testing.T, stored Session) (*Manager, *atomic.Int32) {
	t.Helper()
	var renewals atomic.Int32
	var mu sync.Mutex
	valid := stored.RefreshToken
	m := &Manager{
		Store:    &lockedStore{},
		LockPath: filepath.Join(t.TempDir(), "lock", "renew.lock"),
		Now:      func() time.Time { return now },
	}
	m.Renew = func(ctx context.Context, refreshToken string) (Session, error) {
		mu.Lock()
		defer mu.Unlock()
		if refreshToken != valid {
			return Session{}, ctlkit.Authf("the refresh token was used before")
		}
		n := renewals.Add(1)
		valid = fmt.Sprintf("refresh-%d", n)
		return Session{
			AccessToken:    fmt.Sprintf("access-%d", n),
			AccessExpires:  now.Add(24 * time.Hour),
			RefreshToken:   valid,
			RefreshExpires: now.Add(90 * 24 * time.Hour),
		}, nil
	}
	if stored.AccessToken != "" {
		if err := m.Save(stored); err != nil {
			t.Fatal(err)
		}
	}
	return m, &renewals
}

func TestAFreshTokenIsUsedAsItIs(t *testing.T) {
	m, renewals := manager(t, Session{AccessToken: "access-0", AccessExpires: now.Add(time.Hour), RefreshToken: "refresh-0"})
	token, err := m.Token(context.Background())
	if err != nil || token != "access-0" || renewals.Load() != 0 {
		t.Errorf("token = %q, err = %v, renewals = %d", token, err, renewals.Load())
	}
}

func TestATokenAboutToRunOutIsRenewedAndTheNewPairStored(t *testing.T) {
	for _, left := range []time.Duration{4 * time.Minute, -time.Hour} {
		m, renewals := manager(t, Session{AccessToken: "access-0", AccessExpires: now.Add(left), RefreshToken: "refresh-0", RefreshExpires: now.Add(time.Hour)})
		token, err := m.Token(context.Background())
		stored, _ := m.Load()
		if err != nil || token != "access-1" || renewals.Load() != 1 || stored.RefreshToken != "refresh-1" || !stored.RefreshExpires.Equal(now.Add(90*24*time.Hour)) {
			t.Errorf("%v left: token = %q, err = %v, stored = %+v", left, token, err, stored)
		}
	}
}

// The service takes a refresh token once. Runs that renew at the same time
// must share one renewal, or all but one of them are logged out.
func TestRunsAtTheSameTimeRenewOnce(t *testing.T) {
	m, renewals := manager(t, Session{AccessToken: "access-0", AccessExpires: now.Add(-time.Minute), RefreshToken: "refresh-0"})
	var wg sync.WaitGroup
	tokens := make([]string, 8)
	errs := make([]error, 8)
	for i := range tokens {
		wg.Go(func() { tokens[i], errs[i] = m.Token(context.Background()) })
	}
	wg.Wait()
	for i := range tokens {
		if errs[i] != nil || tokens[i] != "access-1" {
			t.Errorf("run %d: token = %q, err = %v", i, tokens[i], errs[i])
		}
	}
	if renewals.Load() != 1 {
		t.Errorf("renewals = %d, want 1", renewals.Load())
	}
}

func TestRefreshRenewsATokenTheServiceRefused(t *testing.T) {
	m, renewals := manager(t, Session{AccessToken: "access-0", AccessExpires: now.Add(time.Hour), RefreshToken: "refresh-0"})
	token, err := m.Refresh(context.Background(), "access-0")
	if err != nil || token != "access-1" || renewals.Load() != 1 {
		t.Errorf("token = %q, err = %v, renewals = %d", token, err, renewals.Load())
	}

	// Another run renewed in the meantime, so the refused token is not the
	// stored one any more, and no second renewal is due.
	token, err = m.Refresh(context.Background(), "access-0")
	if err != nil || token != "access-1" || renewals.Load() != 1 {
		t.Errorf("after another run renewed: token = %q, err = %v, renewals = %d", token, err, renewals.Load())
	}
}

func TestALoginThatCannotBeRenewedHasRunOut(t *testing.T) {
	tests := map[string]Session{
		"no refresh token":          {AccessToken: "access-0", AccessExpires: now.Add(-time.Minute)},
		"refresh token has run out": {AccessToken: "access-0", AccessExpires: now.Add(-time.Minute), RefreshToken: "refresh-0", RefreshExpires: now.Add(-time.Second)},
		"refresh token is refused":  {AccessToken: "access-0", AccessExpires: now.Add(-time.Minute), RefreshToken: "refresh-0"},
	}
	for name, stored := range tests {
		m, _ := manager(t, stored)
		if name == "refresh token is refused" {
			m.Renew = func(context.Context, string) (Session, error) { return Session{}, ctlkit.Authf("Service: HTTP 401") }
		}
		_, err := m.Token(context.Background())
		if !errors.Is(err, ErrExpired) || ctlkit.KindOf(err) != ctlkit.KindAuth {
			t.Errorf("%s: err = %v", name, err)
		}
		if after, _ := m.Load(); after.AccessToken != "access-0" {
			t.Errorf("%s: the stored login was changed to %+v", name, after)
		}
	}
}

func TestAFailedRenewalThatIsNotARefusalKeepsTheLogin(t *testing.T) {
	m, _ := manager(t, Session{AccessToken: "access-0", AccessExpires: now.Add(-time.Minute), RefreshToken: "refresh-0"})
	m.Renew = func(context.Context, string) (Session, error) {
		return Session{}, ctlkit.Errorf(ctlkit.KindUnavailable, "Service: HTTP 503")
	}
	_, err := m.Token(context.Background())
	if errors.Is(err, ErrExpired) || ctlkit.KindOf(err) != ctlkit.KindUnavailable {
		t.Errorf("err = %v", err)
	}
}

func TestNothingStoredIsNotLoggedIn(t *testing.T) {
	m, _ := manager(t, Session{})
	if _, err := m.Token(context.Background()); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("err = %v", err)
	}
}

func TestABareTokenFromAnOlderVersionStillWorks(t *testing.T) {
	m, _ := manager(t, Session{})
	if err := m.Store.Set("a-bare-token"); err != nil {
		t.Fatal(err)
	}
	if token, err := m.Token(context.Background()); err != nil || token != "a-bare-token" {
		t.Errorf("token = %q, err = %v", token, err)
	}
	// The service refused it, and there is nothing to renew it with.
	if _, err := m.Refresh(context.Background(), "a-bare-token"); !errors.Is(err, ErrExpired) {
		t.Errorf("refresh: err = %v", err)
	}
}

func TestAServiceThatKeepsItsRefreshTokenKeepsIt(t *testing.T) {
	m, _ := manager(t, Session{AccessToken: "access-0", AccessExpires: now.Add(-time.Minute), RefreshToken: "refresh-0", RefreshExpires: now.Add(time.Hour)})
	m.Renew = func(context.Context, string) (Session, error) {
		return Session{AccessToken: "access-1", AccessExpires: now.Add(time.Hour)}, nil
	}
	if _, err := m.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stored, _ := m.Load(); stored.RefreshToken != "refresh-0" || !stored.RefreshExpires.Equal(now.Add(time.Hour)) {
		t.Errorf("stored = %+v", stored)
	}
}

func TestClearLogsOut(t *testing.T) {
	m, _ := manager(t, Session{AccessToken: "access-0"})
	if err := m.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Load(); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("err = %v", err)
	}
}

func TestExtraSurvivesARenewalThatReturnsNone(t *testing.T) {
	m, _ := manager(t, Session{})
	m.Renew = nil
	m.RenewFrom = func(ctx context.Context, stored Session) (Session, error) {
		if stored.Extra["sid"] != "s1" || stored.RefreshToken != "r1" {
			t.Errorf("RenewFrom got %+v", stored)
		}
		return Session{AccessToken: "a2", AccessExpires: now.Add(time.Hour)}, nil
	}
	if err := m.Save(Session{AccessToken: "a1", AccessExpires: now.Add(-time.Minute), RefreshToken: "r1", Extra: map[string]string{"sid": "s1"}}); err != nil {
		t.Fatal(err)
	}
	token, err := m.Token(context.Background())
	got, _ := m.Load()
	if err != nil || token != "a2" || got.RefreshToken != "r1" || got.Extra["sid"] != "s1" {
		t.Errorf("token = %q, err = %v, stored = %+v", token, err, got)
	}
}

func TestJWTExpiry(t *testing.T) {
	// The payload is {"exp":1791806400}.
	if got := JWTExpiry("e30.eyJleHAiOjE3OTE4MDY0MDB9.sig"); !got.Equal(time.Unix(1791806400, 0)) {
		t.Errorf("JWTExpiry = %v", got)
	}
	for _, token := range []string{"", "opaque", "a.b.c", "e30.e30.sig"} {
		if got := JWTExpiry(token); !got.IsZero() {
			t.Errorf("JWTExpiry(%q) = %v", token, got)
		}
	}
}
