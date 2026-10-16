// Package keystore keeps one secret between runs: a token, a key or a
// password. It lives in the operating system's credential store, never in a
// file.
package keystore

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

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
//
// The macOS keychain, through go-keyring, refuses a value of more than
// about 3000 bytes, and a login cookie can be longer. A long secret is kept
// in parts: the entry named account holds how many, and the parts are in
// account.1, account.2 and so on. A secret that fits is kept in one entry,
// as before.
func Keyring(service, account string) Store {
	return chunked{open: func(part int) Store {
		if part == 0 {
			return systemKeyring{service: service, account: account}
		}
		return systemKeyring{service: service, account: account + "." + strconv.Itoa(part)}
	}}
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

// partSize keeps each part well under the keychain's limit after go-keyring
// has encoded it.
const partSize = 1800

// partsHead starts the entry that says how many parts a secret has.
const partsHead = "ctlkit-parts:"

// chunked keeps a secret that is too long for one entry in several. open
// returns the entry for a part: 0 is the head.
type chunked struct{ open func(part int) Store }

func (c chunked) Get() (string, error) {
	head, err := c.open(0).Get()
	if err != nil || !strings.HasPrefix(head, partsHead) {
		return head, err
	}
	n, err := strconv.Atoi(strings.TrimPrefix(head, partsHead))
	if err != nil || n < 1 {
		return "", errors.New("the keyring entry is damaged: log out and log in again")
	}
	var b strings.Builder
	for i := 1; i <= n; i++ {
		part, err := c.open(i).Get()
		if errors.Is(err, ErrNotFound) {
			return "", fmt.Errorf("the keyring entry is missing part %d of %d: log out and log in again", i, n)
		}
		if err != nil {
			return "", err
		}
		b.WriteString(part)
	}
	return b.String(), nil
}

func (c chunked) Set(secret string) error {
	old := c.parts()
	n := 0
	if len(secret) <= partSize {
		if err := c.open(0).Set(secret); err != nil {
			return err
		}
	} else {
		n = (len(secret) + partSize - 1) / partSize
		for i := 1; i <= n; i++ {
			if err := c.open(i).Set(secret[(i-1)*partSize : min(i*partSize, len(secret))]); err != nil {
				return err
			}
		}
		// The head is written last, so that a reader never counts parts
		// that are not there yet.
		if err := c.open(0).Set(partsHead + strconv.Itoa(n)); err != nil {
			return err
		}
	}
	for i := n + 1; i <= old; i++ {
		_ = c.open(i).Delete()
	}
	return nil
}

func (c chunked) Delete() error {
	n := c.parts()
	if err := c.open(0).Delete(); err != nil {
		return err
	}
	for i := 1; i <= n; i++ {
		if err := c.open(i).Delete(); err != nil {
			return err
		}
	}
	return nil
}

// parts is how many parts the stored secret has, or 0 when it is in one
// entry or not there.
func (c chunked) parts() int {
	head, err := c.open(0).Get()
	if err != nil || !strings.HasPrefix(head, partsHead) {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimPrefix(head, partsHead))
	return n
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
