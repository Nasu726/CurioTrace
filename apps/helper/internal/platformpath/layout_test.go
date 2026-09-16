package platformpath

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestEnsureRejectsRenamedManagedSubdirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "curiotrace")
	paths := Paths{
		Root:         root,
		Observations: filepath.Join(root, "content"),
		Authority:    filepath.Join(root, "authority"),
	}
	if err := Ensure(paths); !errors.Is(err, ErrUnsafeStoragePath) {
		t.Fatalf("renamed observations directory was not rejected: %v", err)
	}
}
