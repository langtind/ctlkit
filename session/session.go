// Package session keeps a login alive between runs. It stores an access
// token with the refresh token that renews it, and renews when the access
// token is about to run out, so that a tool stays logged in for as long as
// the service lets a refresh token live.
//
// A tool is often run by an agent, several at a time. A service that hands
// out a new refresh token on every renewal accepts the old one only once,
// so two runs that renewed at the same moment would log each other out.
// Manager renews under a lock file for that reason.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/langtind/ctlkit"
	"github.com/langtind/ctlkit/keystore"
)

var (
	// ErrNotLoggedIn means nothing is stored.
	ErrNotLoggedIn = ctlkit.Authf("not logged in")

	// ErrExpired means the stored login can no longer be renewed, and the
	// user has to log in again.
	ErrExpired = ctlkit.Authf("the login has run out")
)

// Session is one login. A zero expiry means the service did not say.
type Session struct {
	AccessToken    string    `json:"accessToken"`
	AccessExpires  time.Time `json:"accessExpires,omitzero"`
	RefreshToken   string    `json:"refreshToken,omitempty"`
	RefreshExpires time.Time `json:"refreshExpires,omitzero"`

	// Extra holds what a login has besides the two tokens, such as a
	// session id that never changes. A renewal that returns none keeps it.
	Extra map[string]string `json:"extra,omitempty"`
}

// Manager stores a session and renews it.
type Manager struct {
	Store keystore.Store

	// Renew trades a refresh token for a new session at the service. A
	// service that has no refresh tokens leaves it nil.
	Renew func(ctx context.Context, refreshToken string) (Session, error)

	// RenewFrom is Renew for a service that needs more than the refresh
	// token to renew, such as a cookie kept in Extra. It gets the whole
	// stored session. When both are set, RenewFrom is used.
	RenewFrom func(ctx context.Context, stored Session) (Session, error)

	// LockPath is the file one run holds while it renews. See LockFile.
	LockPath string

	// Margin is how long before the access token runs out it is renewed.
	// Zero means five minutes.
	Margin time.Duration

	// Now is the clock. Nil means time.Now.
	Now func() time.Time
}

// LockFile returns the lock file for a program, in the user's cache directory.
func LockFile(program string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, program, "renew.lock")
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) fresh(s Session) bool {
	margin := m.Margin
	if margin == 0 {
		margin = 5 * time.Minute
	}
	return s.AccessExpires.IsZero() || m.now().Add(margin).Before(s.AccessExpires)
}

// Save stores a session, as login does.
func (m *Manager) Save(s Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return m.Store.Set(string(data))
}

// Clear removes the stored session, as logout does.
func (m *Manager) Clear() error { return m.Store.Delete() }

// Load returns the stored session without renewing it.
func (m *Manager) Load() (Session, error) {
	raw, err := m.Store.Get()
	switch {
	case errors.Is(err, keystore.ErrNotFound) || (err == nil && raw == ""):
		return Session{}, ErrNotLoggedIn
	case err != nil:
		return Session{}, fmt.Errorf("read the login from the keyring: %w", err)
	}
	var s Session
	// A value that is not a session is a bare token, as an older version
	// of a tool stored it.
	if !strings.HasPrefix(raw, "{") || json.Unmarshal([]byte(raw), &s) != nil || s.AccessToken == "" {
		return Session{AccessToken: raw}, nil
	}
	return s, nil
}

// Token returns an access token that is good for a request. It renews the
// session first when the token has run out or is about to.
func (m *Manager) Token(ctx context.Context) (string, error) {
	s, err := m.Load()
	if err != nil {
		return "", err
	}
	if m.fresh(s) {
		return s.AccessToken, nil
	}
	return m.renew(ctx, s.AccessToken, false)
}

// Refresh renews the session although the access token looks good. A tool
// calls it when the service answered 401 to stale, the token it sent.
func (m *Manager) Refresh(ctx context.Context, stale string) (string, error) {
	return m.renew(ctx, stale, true)
}

// renew gets a new session under the lock. Another run may have renewed
// while this one waited for the lock, and then its token is used.
func (m *Manager) renew(ctx context.Context, stale string, force bool) (string, error) {
	unlock, err := m.lock(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()

	s, err := m.Load()
	if err != nil {
		return "", err
	}
	if s.AccessToken != stale && m.fresh(s) {
		return s.AccessToken, nil
	}
	if !force && m.fresh(s) {
		return s.AccessToken, nil
	}
	if (m.Renew == nil && m.RenewFrom == nil) || s.RefreshToken == "" {
		return "", ErrExpired
	}
	if !s.RefreshExpires.IsZero() && !m.now().Before(s.RefreshExpires) {
		return "", ErrExpired
	}

	var renewed Session
	if m.RenewFrom != nil {
		renewed, err = m.RenewFrom(ctx, s)
	} else {
		renewed, err = m.Renew(ctx, s.RefreshToken)
	}
	if err != nil {
		// Only a clear no from the service means the login is over. A
		// network failure says nothing about it.
		if ctlkit.KindOf(err) == ctlkit.KindAuth {
			return "", fmt.Errorf("%w: %w", ErrExpired, err)
		}
		return "", fmt.Errorf("renew the login: %w", err)
	}
	if renewed.AccessToken == "" {
		return "", errors.New("renew the login: the service gave no access token")
	}
	// A service that does not rotate refresh tokens leaves the old one valid.
	if renewed.RefreshToken == "" {
		renewed.RefreshToken, renewed.RefreshExpires = s.RefreshToken, s.RefreshExpires
	}
	if renewed.Extra == nil {
		renewed.Extra = s.Extra
	}
	if err := m.Save(renewed); err != nil {
		return "", fmt.Errorf("store the renewed login in the keyring: %w", err)
	}
	return renewed.AccessToken, nil
}

const (
	lockWait  = 20 * time.Second
	lockStale = 60 * time.Second
	lockPoll  = 50 * time.Millisecond
)

// lock takes the lock file and returns the function that gives it back. A
// lock left behind by a run that died is taken over when it is a minute old.
func (m *Manager) lock(ctx context.Context) (func(), error) {
	if m.LockPath == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(m.LockPath), 0o700); err != nil {
		return nil, fmt.Errorf("renew the login: %w", err)
	}
	deadline := time.Now().Add(lockWait)
	for {
		file, err := os.OpenFile(m.LockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = file.Close()
			return func() { _ = os.Remove(m.LockPath) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("renew the login: %w", err)
		}
		if info, statErr := os.Stat(m.LockPath); statErr == nil && time.Since(info.ModTime()) > lockStale {
			_ = os.Remove(m.LockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, ctlkit.Errorf(ctlkit.KindUnavailable, "renew the login: another run has held %s for too long", m.LockPath)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(lockPoll):
		}
	}
}
