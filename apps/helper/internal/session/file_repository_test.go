package session

import (
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"runtime"
	"testing"
)

func TestFileSnapshotRepositoryReopensLatestSnapshot(t *testing.T) {
	repo, err := NewFileSnapshotRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	recording := Snapshot{State: Recording, SessionID: testSessionID("d"), RecordingEpoch: 1}
	paused := Snapshot{State: Paused, SessionID: recording.SessionID, RecordingEpoch: 2}
	if err := repo.Save(context.Background(), recording); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(context.Background(), paused); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewFileSnapshotRepository(repo.root)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := reopened.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !found || got != paused {
		t.Fatalf("unexpected latest snapshot: found=%v got=%+v want=%+v", found, got, paused)
	}
}

func TestFileSnapshotRepositoryMissingStateIsNotCorruption(t *testing.T) {
	repo, err := NewFileSnapshotRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if found || got != (Snapshot{}) {
		t.Fatalf("missing state unexpectedly produced snapshot: found=%v got=%+v", found, got)
	}
}

func TestFileSnapshotRepositoryRepairsOnlyPartialTail(t *testing.T) {
	repo, err := NewFileSnapshotRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := Snapshot{State: Recording, SessionID: testSessionID("e"), RecordingEpoch: 3}
	if err := repo.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(repo.path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(repo.path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte{0, 0, 0, 20}); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	got, found, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !found || got != want {
		t.Fatalf("partial tail recovery changed latest snapshot: found=%v got=%+v", found, got)
	}
	after, err := os.Stat(repo.path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() {
		t.Fatalf("partial tail was not truncated: before=%d after=%d", before.Size(), after.Size())
	}
}

func TestFileSnapshotRepositoryRejectsChecksumCorruption(t *testing.T) {
	repo, err := NewFileSnapshotRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(context.Background(), Snapshot{State: Recording, SessionID: testSessionID("f"), RecordingEpoch: 4}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(repo.path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 0xff
	if err := os.WriteFile(repo.path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.Load(context.Background()); !errors.Is(err, ErrCorruptStateLog) {
		t.Fatalf("checksum corruption was not rejected: %v", err)
	}
}

func TestFileSnapshotRepositoryRejectsUnknownSnapshotFields(t *testing.T) {
	repo, err := NewFileSnapshotRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"version":1,"state":"RECORDING","session_id":"ses_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","recording_epoch":2,"unexpected":true}`)
	writeRawSnapshotLog(t, repo.path, payload)
	if _, _, err := repo.Load(context.Background()); !errors.Is(err, ErrCorruptStateLog) {
		t.Fatalf("unknown field was not rejected: %v", err)
	}
}

func TestFileSnapshotRepositoryRejectsInvalidSemanticSnapshot(t *testing.T) {
	repo, err := NewFileSnapshotRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"version":1,"state":"RECORDING","session_id":"","recording_epoch":0}`)
	writeRawSnapshotLog(t, repo.path, payload)
	if _, _, err := repo.Load(context.Background()); !errors.Is(err, ErrCorruptStateLog) {
		t.Fatalf("invalid semantic snapshot was not rejected: %v", err)
	}
}

func TestFileSnapshotRepositoryRejectsTrailingJSONValue(t *testing.T) {
	repo, err := NewFileSnapshotRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"version":1,"state":"RECORDING","session_id":"ses_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","recording_epoch":2} {}`)
	writeRawSnapshotLog(t, repo.path, payload)
	if _, _, err := repo.Load(context.Background()); !errors.Is(err, ErrCorruptStateLog) {
		t.Fatalf("trailing JSON value was not rejected: %v", err)
	}
}

func TestFileSnapshotRepositoryProtectsManagedFilesOnPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}
	root := t.TempDir()
	repo, err := NewFileSnapshotRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(context.Background(), Snapshot{State: Recording, SessionID: testSessionID("1"), RecordingEpoch: 1}); err != nil {
		t.Fatal(err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := rootInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("unexpected state directory mode: %o", got)
	}
	fileInfo, err := os.Stat(repo.path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("unexpected state file mode: %o", got)
	}
}

func writeRawSnapshotLog(t *testing.T, path string, payload []byte) {
	t.Helper()
	frame := make([]byte, len(snapshotLogMagic)+8+len(payload))
	copy(frame, snapshotLogMagic[:])
	offset := len(snapshotLogMagic)
	binary.BigEndian.PutUint32(frame[offset:offset+4], uint32(len(payload)))
	binary.BigEndian.PutUint32(frame[offset+4:offset+8], crc32.Checksum(payload, snapshotCRCTable))
	copy(frame[offset+8:], payload)
	if err := os.WriteFile(path, frame, 0o600); err != nil {
		t.Fatal(err)
	}
}
