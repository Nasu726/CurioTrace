//go:build !linux && !darwin && !windows

package profilelock

import "os"

func lockFile(*os.File) error {
	return ErrUnsupportedLock
}

func unlockFile(*os.File) error {
	return nil
}
