package keystore

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The variables that choose where secrets are kept. They are the same for
// every tool, so one container sets them once.
const (
	// EnvKind is "keyring", the default, or "file".
	EnvKind = "CTLKIT_KEYSTORE"
	// EnvDir is the directory of the file store. Setting it chooses the
	// file store.
	EnvDir = "CTLKIT_KEYSTORE_DIR"
	// EnvKey is the key the file store encrypts with: 32 random bytes in
	// base64, as "openssl rand -base64 32" prints them.
	EnvKey = "CTLKIT_KEYSTORE_KEY"
)

// getenv is os.Getenv. Tests replace it.
var getenv = os.Getenv

// Kind is where secrets are kept.
type Kind string

const (
	SystemKeyring Kind = "keyring"
	File          Kind = "file"
)

// Config is where a program's secrets are kept.
type Config struct {
	Kind Kind
	// Dir is the directory of the file store, one directory per program.
	Dir string
	key []byte
}

// ConfigFrom reads the store a program uses from the environment. The file
// store needs a key: a secret is never written to disk in the clear.
func ConfigFrom(getenv func(string) string, program string) (Config, error) {
	kind := Kind(strings.TrimSpace(getenv(EnvKind)))
	dir := strings.TrimSpace(getenv(EnvDir))
	switch {
	case kind == SystemKeyring || (kind == "" && dir == ""):
		return Config{Kind: SystemKeyring}, nil
	case kind != "" && kind != File:
		return Config{}, fmt.Errorf("%s is %q: use %q or %q", EnvKind, kind, SystemKeyring, File)
	}

	if dir == "" {
		state := strings.TrimSpace(getenv("XDG_STATE_HOME"))
		if state == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return Config{}, fmt.Errorf("the file store: no home directory, so set %s: %w", EnvDir, err)
			}
			state = filepath.Join(home, ".local", "state")
		}
		dir = filepath.Join(state, program)
	} else {
		dir = filepath.Join(dir, program)
	}
	if !filepath.IsAbs(dir) {
		return Config{}, fmt.Errorf("the file store: %s must be an absolute path", EnvDir)
	}

	key, err := parseKey(getenv(EnvKey))
	if err != nil {
		return Config{}, err
	}
	return Config{Kind: File, Dir: dir, key: key}, nil
}

func parseKey(text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf(`the file store needs a key in %s: make one with "openssl rand -base64 32" and keep it where the other secrets of the machine are`, EnvKey)
	}
	key, err := base64.StdEncoding.DecodeString(text)
	if err != nil || len(key) != keySize {
		return nil, fmt.Errorf(`%s is not 32 bytes in base64: make one with "openssl rand -base64 32"`, EnvKey)
	}
	return key, nil
}

// Describe says where the secrets of a program are kept, for status and
// login. It names no secret.
func Describe(program string) string {
	config, err := ConfigFrom(getenv, program)
	switch {
	case err != nil:
		return "nowhere yet: " + err.Error()
	case config.Kind == File:
		return "an encrypted file in " + config.Dir
	}
	return "the system keyring"
}

// LockDir is the directory of the file store when a program uses one, and
// "" when it uses the system keyring. session.LockFile puts its lock there,
// so that every run that shares the files also shares the lock.
func LockDir(program string) string {
	config, err := ConfigFrom(getenv, program)
	if err != nil || config.Kind != File {
		return ""
	}
	return config.Dir
}

// DefaultProfile is the profile used when none is chosen. Its secrets keep
// the names they had before there were profiles.
const DefaultProfile = "default"

// Profile is a name that keeps one login apart from another of the same
// program, such as two people's accounts on one machine.
type Profile string

var profileName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ParseProfile checks a profile name: lower-case letters, digits and
// dashes, at most 32. An empty name is the default profile.
func ParseProfile(name string) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultProfile, nil
	}
	if !profileName.MatchString(name) {
		return "", fmt.Errorf("the profile %q: use lower-case letters, digits and dashes, at most 32", name)
	}
	return Profile(name), nil
}

// ProfileEnv is the variable that chooses the profile of a program:
// COOPCTL_PROFILE for coopctl.
func ProfileEnv(program string) string {
	return strings.ToUpper(strings.ReplaceAll(program, "-", "_")) + "_PROFILE"
}

// ProfileFrom reads the profile a program uses from the environment.
func ProfileFrom(getenv func(string) string, program string) (Profile, error) {
	return ParseProfile(getenv(ProfileEnv(program)))
}

// CurrentProfile is the profile a program uses now, for status. A name that
// does not parse is reported as it is; using the store says what is wrong.
func CurrentProfile(program string) string {
	if p, err := ProfileFrom(getenv, program); err == nil {
		return string(p)
	}
	return getenv(ProfileEnv(program))
}

// entry is the name of a secret under the profile. The default profile
// keeps the bare name.
func (p Profile) entry(account string) string {
	if p == DefaultProfile {
		return account
	}
	return "profiles/" + string(p) + "/" + account
}
