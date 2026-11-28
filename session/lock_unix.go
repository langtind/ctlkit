//go:build unix

package session

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive flock on the file without waiting. It is false
// when another open file holds it.
func tryLock(file *os.File) (bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func unlock(file *os.File) error { return syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
