package platformpath

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveWindowsUsesLocalAppDataOnly(t *testing.T) {
	env := mapEnvironment(map[string]string{
		"LOCALAPPDATA": filepath.Join(string(filepath.Separator), "local-app-data"),
		"APPDATA":      filepath.Join(string(filepath.Separator), "roaming-app-data"),
	})
	paths, err := Resolve("windows", Environment{Getenv: env, Home: "/home/ignored"})
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join(string(filepath.Separator), "local-app-data", "CurioTrace")
	if paths.Root != wantRoot {
		t.Fatalf("Windows root mismatch: got=%q want=%q", paths.Root, wantRoot)
	}
	assertSubdirs(t, paths)
}

func TestResolveWindowsRequiresLocalAppData(t *testing.T) {
	_, err := Resolve("windows", Environment{
		Getenv: mapEnvironment(map[string]string{"APPDATA": "/roaming-only"}),
		Home:   "/home/ignored",
	})
	if !errors.Is(err, ErrPlatformPathMissing) {
		t.Fatalf("missing LOCALAPPDATA was not rejected: %v", err)
	}
}

func TestResolveDarwinUsesApplicationSupport(t *testing.T) {
	paths, err := Resolve("darwin", Environment{Home: "/Users/test"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/Users/test", "Library", "Application Support", "CurioTrace")
	if paths.Root != want {
		t.Fatalf("macOS root mismatch: got=%q want=%q", paths.Root, want)
	}
	assertSubdirs(t, paths)
}

func TestResolveLinuxPrefersAbsoluteXDGDataHome(t *testing.T) {
	paths, err := Resolve("linux", Environment{
		Getenv: mapEnvironment(map[string]string{"XDG_DATA_HOME": "/xdg/data"}),
		Home:   "/home/test",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/xdg/data", "curiotrace")
	if paths.Root != want {
		t.Fatalf("Linux XDG root mismatch: got=%q want=%q", paths.Root, want)
	}
}

func TestResolveLinuxIgnoresRelativeXDGDataHome(t *testing.T) {
	paths, err := Resolve("linux", Environment{
		Getenv: mapEnvironment(map[string]string{"XDG_DATA_HOME": "relative/data"}),
		Home:   "/home/test",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/home/test", ".local", "share", "curiotrace")
	if paths.Root != want {
		t.Fatalf("relative XDG path was not ignored: got=%q want=%q", paths.Root, want)
	}
}

func TestResolveLinuxRequiresAbsoluteBaseOrHome(t *testing.T) {
	_, err := Resolve("linux", Environment{
		Getenv: mapEnvironment(map[string]string{"XDG_DATA_HOME": "relative/data"}),
	})
	if !errors.Is(err, ErrPlatformPathMissing) {
		t.Fatalf("missing safe Linux base was not rejected: %v", err)
	}
}

func TestResolveRejectsUnsupportedPlatform(t *testing.T) {
	_, err := Resolve("freebsd", Environment{Home: "/home/test"})
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("unsupported platform was not rejected: %v", err)
	}
}

func TestEnsureCreatesSeparatedPrivateDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "curiotrace")
	paths := Paths{
		Root:         root,
		Observations: filepath.Join(root, "observations"),
		Authority:    filepath.Join(root, "authority"),
	}
	if err := Ensure(paths); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.Root, paths.Observations, paths.Authority} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("managed path is not a real directory: %q mode=%v", path, info.Mode())
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
			t.Fatalf("unexpected managed directory permissions for %q: %o", path, info.Mode().Perm())
		}
	}
}

func TestEnsureRejectsManagedSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require Windows developer/admin privileges")
	}
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "root-link")
	if err := os.Symlink(target, root); err != nil {
		t.Fatal(err)
	}
	paths := Paths{
		Root:         root,
		Observations: filepath.Join(root, "observations"),
		Authority:    filepath.Join(root, "authority"),
	}
	if err := Ensure(paths); !errors.Is(err, ErrUnsafeStoragePath) {
		t.Fatalf("managed root symlink was not rejected: %v", err)
	}
}

func TestEnsureRejectsLayoutOutsideRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	paths := Paths{
		Root:         root,
		Observations: filepath.Join(base, "outside"),
		Authority:    filepath.Join(root, "authority"),
	}
	if err := Ensure(paths); !errors.Is(err, ErrUnsafeStoragePath) {
		t.Fatalf("outside observations directory was not rejected: %v", err)
	}
}

func TestEnsureRejectsFileAtManagedDirectory(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "curiotrace")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	observations := filepath.Join(root, "observations")
	if err := os.WriteFile(observations, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := Paths{Root: root, Observations: observations, Authority: filepath.Join(root, "authority")}
	if err := Ensure(paths); !errors.Is(err, ErrUnsafeStoragePath) {
		t.Fatalf("managed file path was not rejected: %v", err)
	}
}

func assertSubdirs(t *testing.T, paths Paths) {
	t.Helper()
	if paths.Observations != filepath.Join(paths.Root, "observations") {
		t.Fatalf("observation path escaped root: %#v", paths)
	}
	if paths.Authority != filepath.Join(paths.Root, "authority") {
		t.Fatalf("authority path escaped root: %#v", paths)
	}
}

func mapEnvironment(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}
