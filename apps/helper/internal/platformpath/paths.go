package platformpath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var (
	ErrUnsupportedPlatform = errors.New("unsupported storage platform")
	ErrPlatformPathMissing = errors.New("required platform storage path is unavailable")
	ErrUnsafeStoragePath   = errors.New("unsafe storage path")
)

// Paths describes the application-private filesystem roots owned by the
// CurioTrace helper. Secret encryption keys are deliberately absent: they live
// in the OS credential store through the SystemKeyProvider adapter.
type Paths struct {
	Root         string
	Observations string
	Authority    string
}

type Environment struct {
	Getenv func(string) string
	Home   string
}

func Current() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return Resolve(runtime.GOOS, Environment{
		Getenv: os.Getenv,
		Home:   home,
	})
}

// Resolve is side-effect free so path policy can be tested for supported OS
// choices without touching the filesystem. Ensure must be called separately
// before the returned paths are used.
func Resolve(goos string, environment Environment) (Paths, error) {
	getenv := environment.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	var root string
	switch goos {
	case "windows":
		base := getenv("LOCALAPPDATA")
		if base == "" {
			return Paths{}, fmt.Errorf("%w: LOCALAPPDATA", ErrPlatformPathMissing)
		}
		root = filepath.Join(base, "CurioTrace")
	case "darwin":
		if environment.Home == "" {
			return Paths{}, fmt.Errorf("%w: user home", ErrPlatformPathMissing)
		}
		root = filepath.Join(environment.Home, "Library", "Application Support", "CurioTrace")
	case "linux":
		base := getenv("XDG_DATA_HOME")
		if base != "" && !filepath.IsAbs(base) {
			// The XDG Base Directory specification requires an absolute path;
			// ignore a relative value rather than writing relative to cwd.
			base = ""
		}
		if base == "" {
			if environment.Home == "" {
				return Paths{}, fmt.Errorf("%w: XDG_DATA_HOME and user home", ErrPlatformPathMissing)
			}
			base = filepath.Join(environment.Home, ".local", "share")
		}
		root = filepath.Join(base, "curiotrace")
	default:
		return Paths{}, ErrUnsupportedPlatform
	}

	root = filepath.Clean(root)
	if root == "." || root == string(filepath.Separator) || !filepath.IsAbs(root) {
		return Paths{}, fmt.Errorf("%w: root %q", ErrUnsafeStoragePath, root)
	}
	return Paths{
		Root:         root,
		Observations: filepath.Join(root, "observations"),
		Authority:    filepath.Join(root, "authority"),
	}, nil
}

// Ensure creates and validates CurioTrace-owned directories. A final managed
// directory that is a symlink is rejected so an existing link cannot silently
// redirect durable browsing data. This is not a claim of full filesystem
// anti-TOCTOU protection against a hostile local process.
func Ensure(paths Paths) error {
	if err := validateLayout(paths); err != nil {
		return err
	}
	for _, path := range []string{paths.Root, paths.Observations, paths.Authority} {
		if err := ensureDirectory(path); err != nil {
			return err
		}
	}
	return nil
}

func validateLayout(paths Paths) error {
	if paths.Root == "" || paths.Observations == "" || paths.Authority == "" {
		return ErrUnsafeStoragePath
	}
	root := filepath.Clean(paths.Root)
	observations := filepath.Clean(paths.Observations)
	authority := filepath.Clean(paths.Authority)
	if root == "." || root == string(filepath.Separator) || !filepath.IsAbs(root) {
		return ErrUnsafeStoragePath
	}
	if observations != filepath.Join(root, "observations") || authority != filepath.Join(root, "authority") {
		return ErrUnsafeStoragePath
	}
	return nil
}

func ensureDirectory(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: managed directory %q", ErrUnsafeStoragePath, path)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("protect storage directory %q: %w", path, err)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect storage directory %q: %w", path, err)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create storage directory %q: %w", path, err)
	}
	info, err = os.Lstat(path)
	if err != nil {
		return fmt.Errorf("verify storage directory %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: managed directory %q", ErrUnsafeStoragePath, path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("protect storage directory %q: %w", path, err)
	}
	return nil
}
