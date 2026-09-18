package profilelock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrInvalidLockRoot = errors.New("invalid profile lock root")
	ErrUnsafeLockPath  = errors.New("unsafe profile lock path")
	ErrProfileLocked   = errors.New("CurioTrace profile is already owned by another helper")
	ErrUnsupportedLock = errors.New("profile locking is unsupported on this platform")
)

// Lock holds the OS-level exclusive lock for one CurioTrace profile.
//
// The lock file itself may remain after Close or process exit. Ownership is the
// kernel lock on the open file descriptor/handle, not file existence.
type Lock struct {
	mu       sync.Mutex
	file     *os.File
	released bool
}

func Acquire(root string) (*Lock, error) {
	if root == "" {
		return nil, ErrInvalidLockRoot
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("%w: inspect root: %v", ErrInvalidLockRoot, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("%w: root", ErrUnsafeLockPath)
	}

	path := filepath.Join(filepath.Clean(root), "helper.lock")
	if lockInfo, err := os.Lstat(path); err == nil {
		if lockInfo.Mode()&os.ModeSymlink != 0 || !lockInfo.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: helper.lock", ErrUnsafeLockPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect profile lock: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open profile lock: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("protect profile lock: %w", err)
	}
	if err := lockFile(file); err != nil {
		file.Close()
		return nil, err
	}
	return &Lock{file: file}, nil
}

func (l *Lock) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	l.released = true
	if l.file == nil {
		return nil
	}
	unlockErr := unlockFile(l.file)
	closeErr := l.file.Close()
	l.file = nil
	return errors.Join(unlockErr, closeErr)
}
