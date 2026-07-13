//go:build darwin || linux

package state

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

const lockFileName = "state.lock"

const lockPollInterval = 25 * time.Millisecond

// ErrLockTimeout is returned when another Grove process keeps the state lock
// longer than the bounded wait used by Update.
var ErrLockTimeout = errors.New("timed out waiting for state lock")

type stateLock struct {
	file *os.File
}

func acquireLock(path string, timeout time.Duration) (*stateLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &stateLock{file: file}, nil
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			file.Close()
			return nil, err
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			file.Close()
			return nil, fmt.Errorf("%w after %s", ErrLockTimeout, timeout)
		}
		if remaining > lockPollInterval {
			remaining = lockPollInterval
		}
		time.Sleep(remaining)
	}
}

func (l *stateLock) release() {
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
}
