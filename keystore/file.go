package keystore

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const keySize = 32

// magic starts every file the store writes, and says how it was sealed.
var magic = []byte("ctlkit-keystore-1\n")

// fileStore keeps one secret in one file, sealed with AES-256-GCM. The
// program and the name of the secret are bound into the seal, so a file
// moved to another name, or to another program, does not open.
type fileStore struct {
	path string
	key  []byte
	aad  []byte
}

func (c Config) file(program, entry string) fileStore {
	return fileStore{
		path: filepath.Join(c.Dir, filepath.FromSlash(entry)),
		key:  c.key,
		aad:  []byte("ctlkit-keystore\x00" + program + "\x00" + entry),
	}
}

func (f fileStore) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(f.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (f fileStore) Get() (string, error) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("the file store: %w", err)
	}
	return f.open(data)
}

func (f fileStore) open(data []byte) (string, error) {
	aead, err := f.aead()
	if err != nil {
		return "", err
	}
	body, ok := bytes.CutPrefix(data, magic)
	if !ok || len(body) < aead.NonceSize()+aead.Overhead() {
		return "", fmt.Errorf("the file store: %s was not written by ctlkit, or is damaged", f.path)
	}
	nonce, sealed := body[:aead.NonceSize()], body[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, sealed, f.aad)
	if err != nil {
		return "", fmt.Errorf("the file store: %s does not open with the key in %s. It was written with another key, or was changed", f.path, EnvKey)
	}
	return string(plain), nil
}

// Set replaces the file in one step: it writes a new file next to it and
// renames it over the old one, so that a run that dies halfway leaves the
// old secret whole. A refresh token that the service has just replaced
// must never be lost to a torn write.
//
// It refuses to replace a file that the key does not open. That file may
// hold the only good login, written with the right key.
func (f fileStore) Set(secret string) error {
	if data, err := os.ReadFile(f.path); err == nil {
		if _, err := f.open(data); err != nil {
			return fmt.Errorf("%w. Nothing was replaced: set the right key, or log out to remove it", err)
		}
	}
	aead, err := f.aead()
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	data := append(append(bytes.Clone(magic), nonce...), aead.Seal(nil, nonce, []byte(secret), f.aad)...)
	if err := writeAtomic(f.path, data); err != nil {
		return fmt.Errorf("the file store: %w", err)
	}
	return nil
}

func (f fileStore) Delete() error {
	err := os.Remove(f.path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("the file store: %w", err)
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	// The rename is only durable once the directory is written too.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
