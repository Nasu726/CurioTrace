//go:build windows

package profilelock

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	lockFileFailImmediately = 0x00000001
	lockFileExclusiveLock   = 0x00000002
	errorLockViolation      = syscall.Errno(33)
	allBytes                = ^uint32(0)
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

func lockFile(file *os.File) error {
	overlapped := new(syscall.Overlapped)
	result, _, callErr := procLockFileEx.Call(
		uintptr(syscall.Handle(file.Fd())),
		uintptr(lockFileFailImmediately|lockFileExclusiveLock),
		0,
		uintptr(allBytes),
		uintptr(allBytes),
		uintptr(unsafe.Pointer(overlapped)),
	)
	if result != 0 {
		return nil
	}
	if errno, ok := callErr.(syscall.Errno); ok && errno == errorLockViolation {
		return ErrProfileLocked
	}
	return fmt.Errorf("acquire profile lock: %w", callErr)
}

func unlockFile(file *os.File) error {
	overlapped := new(syscall.Overlapped)
	result, _, callErr := procUnlockFileEx.Call(
		uintptr(syscall.Handle(file.Fd())),
		0,
		uintptr(allBytes),
		uintptr(allBytes),
		uintptr(unsafe.Pointer(overlapped)),
	)
	if result != 0 {
		return nil
	}
	return fmt.Errorf("release profile lock: %w", callErr)
}
