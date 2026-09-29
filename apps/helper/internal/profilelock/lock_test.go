package profilelock

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAcquireExcludesSecondOwnerAndReleasesOnClose(t *testing.T) {
	root := t.TempDir()

	first, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	second, err := Acquire(root)
	if second != nil {
		second.Close()
		t.Fatalf("unexpected second lock: %#v", second)
	}
	if !errors.Is(err, ErrProfileLocked) {
		t.Fatalf("expected ErrProfileLocked, got %v", err)
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := Acquire(root)
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	lock, err := Acquire(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
}

func TestAcquireRequiresExistingSafeRoot(t *testing.T) {
	base := t.TempDir()
	missing := filepath.Join(base, "missing")
	if lock, err := Acquire(missing); lock != nil || !errors.Is(err, ErrInvalidLockRoot) {
		t.Fatalf("missing root: lock=%#v err=%v", lock, err)
	}

	fileRoot := filepath.Join(base, "file")
	if err := os.WriteFile(fileRoot, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if lock, err := Acquire(fileRoot); lock != nil || !errors.Is(err, ErrUnsafeLockPath) {
		t.Fatalf("file root: lock=%#v err=%v", lock, err)
	}
}

func TestAcquireRejectsSymlinkRootAndLockFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires environment-specific privileges on Windows")
	}
	base := t.TempDir()

	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	rootLink := filepath.Join(base, "root-link")
	if err := os.Symlink(target, rootLink); err != nil {
		t.Fatal(err)
	}
	if lock, err := Acquire(rootLink); lock != nil || !errors.Is(err, ErrUnsafeLockPath) {
		t.Fatalf("symlink root: lock=%#v err=%v", lock, err)
	}

	root := filepath.Join(base, "safe")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(base, "target.lock")
	if err := os.WriteFile(targetFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetFile, filepath.Join(root, "helper.lock")); err != nil {
		t.Fatal(err)
	}
	if lock, err := Acquire(root); lock != nil || !errors.Is(err, ErrUnsafeLockPath) {
		t.Fatalf("symlink lock file: lock=%#v err=%v", lock, err)
	}
}

func TestLockFileMayRemainButDoesNotRepresentOwnership(t *testing.T) {
	root := t.TempDir()
	lock, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "helper.lock")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file should be reusable, not deleted: %v", err)
	}
	next, err := Acquire(root)
	if err != nil {
		t.Fatalf("persistent lock file was mistaken for ownership: %v", err)
	}
	next.Close()
}

func TestLockFileUsesPrivatePermissionsOnPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits do not express Windows ACLs")
	}
	root := t.TempDir()
	lock, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()

	info, err := os.Stat(filepath.Join(root, "helper.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode=%#o want=0600", got)
	}
}
