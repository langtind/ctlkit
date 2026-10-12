// Package keystore keeps one secret between runs: a token, a key or a
// password. It lives in the operating system's credential store, never in a
// file.
package keystore

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// ErrNotFound means nothing has been stored.
var ErrNotFound = errors.New("nothing stored")

type Store interface {
	Get() (string, error)
	Set(secret string) error
	Delete() error
}

// Keyring returns a store in the system keyring. service is the program
// name, and account says what the secret is, such as "token".
func Keyring(service, account string) Store {
	return systemKeyring{service: service, account: account}
}

type systemKeyring struct{ service, account string }

func (k systemKeyring) Get() (string, error) {
	secret, err := keyring.Get(k.service, k.account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return secret, err
}

func (k systemKeyring) Set(secret string) error {
	return keyring.Set(k.service, k.account, secret)
}

func (k systemKeyring) Delete() error {
	err := keyring.Delete(k.service, k.account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// Memory is a store for tests. Err, when set, is what every Get returns.
type Memory struct {
	Secret string
	Err    error
}

func (m *Memory) Get() (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	if m.Secret == "" {
		return "", ErrNotFound
	}
	return m.Secret, nil
}

func (m *Memory) Set(secret string) error { m.Secret = secret; return nil }
func (m *Memory) Delete() error           { m.Secret = ""; return nil }
