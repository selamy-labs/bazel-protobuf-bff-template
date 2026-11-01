package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Lock is an exclusive flock held by this process. Go opens files close-on-exec, so no child, and
// no daemon a child starts, inherits it: the lock ends with the waiting parent. A capacity lock
// bounds how many runs start. It says nothing about who owns a Bazel server (spec 006 R7).
type Lock struct{ file *os.File }

// Release frees the lock.
func (l *Lock) Release() { _ = l.file.Close() }

func tryLock(path string) (*Lock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, nil
		}
		return nil, err
	}
	return &Lock{file: file}, nil
}

// Acquire takes the first free lock among paths, polling until ctx ends.
func Acquire(ctx context.Context, paths []string) (*Lock, error) {
	for {
		for _, path := range paths {
			lock, err := tryLock(path)
			if err != nil {
				return nil, err
			}
			if lock != nil {
				return lock, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("no capacity slot came free: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// SlotPaths names count slots under dir.
func SlotPaths(dir string, count int) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	paths := make([]string, count)
	for index := range paths {
		paths[index] = filepath.Join(dir, fmt.Sprintf("slot-%d", index))
	}
	return paths, nil
}
