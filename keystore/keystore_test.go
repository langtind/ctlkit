package keystore

import (
	"strings"
	"testing"
)

func memoryParts() (chunked, map[int]*Memory) {
	entries := map[int]*Memory{}
	return chunked{open: func(part int) Store {
		if entries[part] == nil {
			entries[part] = &Memory{}
		}
		return entries[part]
	}}, entries
}

func TestAShortSecretIsOneEntry(t *testing.T) {
	store, entries := memoryParts()
	if err := store.Set("token"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(); err != nil || got != "token" || entries[0].Secret != "token" || len(entries) != 1 {
		t.Errorf("get = %q, %v, entries = %d", got, err, len(entries))
	}
}

func TestALongSecretIsKeptInParts(t *testing.T) {
	store, entries := memoryParts()
	long := strings.Repeat("x", 2*partSize+5)
	if err := store.Set(long); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(); err != nil || got != long || entries[0].Secret != "ctlkit-parts:3" || len(entries[3].Secret) != 5 {
		t.Fatalf("get = %d bytes, %v, head = %q", len(got), err, entries[0].Secret)
	}

	// A shorter secret afterwards leaves no parts behind.
	if err := store.Set("short"); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(); got != "short" || entries[1].Secret != "" || entries[3].Secret != "" {
		t.Errorf("after a shorter secret: %q, parts left = %q %q", got, entries[1].Secret, entries[3].Secret)
	}

	if err := store.Set(long); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(); err != nil || entries[0].Secret != "" || entries[1].Secret != "" || entries[3].Secret != "" {
		t.Errorf("delete: %v", err)
	}
	if _, err := store.Get(); err != ErrNotFound {
		t.Errorf("after delete: %v", err)
	}
}

func TestAMissingPartIsAnError(t *testing.T) {
	store, entries := memoryParts()
	if err := store.Set(strings.Repeat("x", partSize+1)); err != nil {
		t.Fatal(err)
	}
	entries[2].Secret = ""
	if _, err := store.Get(); err == nil || !strings.Contains(err.Error(), "missing part 2 of 2") {
		t.Errorf("err = %v", err)
	}
}
