//go:build linux || darwin

package profilelock

import (
	"fmt"
	"os"
	"syscall"
)

func lockFile(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return nil
	}
	if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
		return ErrProfileLocked
	}
	return fmt.Errorf("acquire profile lock: %w", err)
}

func unlockFile(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("release profile lock: %w", err)
	}
	return nil
}
