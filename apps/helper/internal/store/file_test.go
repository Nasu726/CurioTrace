package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
)

type testJSONCodec struct {
	id string
}

func (c testJSONCodec) ID() string { return c.id }

func (c testJSONCodec) Encode(event observation.ValidatedEvent) ([]byte, error) {
	return json.Marshal(event)
}

func (c testJSONCodec) Decode(record []byte) (observation.ValidatedEvent, error) {
	return observation.DecodeAndValidate(record)
}

func TestFileStorePersistsAcrossReopen(t *testing.T) {
	root := t.TempDir()
	codec := testJSONCodec{id: "test-json-v1"}
	first, err := NewFileStore(root, codec)
	if err != nil {
		t.Fatal(err)
	}
	event1 := testValidatedEvent(t, "ses_reopen", "evt_1", "one")
	event2 := testValidatedEvent(t, "ses_reopen", "evt_2", "two")
	if err := first.Append(context.Background(), event1); err != nil {
		t.Fatal(err)
	}
	if err := first.Append(context.Background(), event2); err != nil {
		t.Fatal(err)
	}

	second, err := NewFileStore(root, codec)
	if err != nil {
		t.Fatal(err)
	}
	events, err := second.ListSession(context.Background(), "ses_reopen")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].EventID() != "evt_1" || events[1].EventID() != "evt_2" {
		t.Fatalf("unexpected durable events: %#v", events)
	}
}

func TestFileStoreRepairsOnlyPartialTail(t *testing.T) {
	root := t.TempDir()
	codec := testJSONCodec{id: "test-json-v1"}
	store, err := NewFileStore(root, codec)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(context.Background(), testValidatedEvent(t, "ses_tail", "evt_1", "one")); err != nil {
		t.Fatal(err)
	}
	path := store.sessionPath("ses_tail")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte{0, 0, 0}); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewFileStore(root, codec)
	if err != nil {
		t.Fatal(err)
	}
	events, err := reopened.ListSession(context.Background(), "ses_tail")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID() != "evt_1" {
		t.Fatalf("partial tail recovery lost valid records: %#v", events)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() {
		t.Fatalf("partial tail was not truncated to last good offset: before=%d after=%d", before.Size(), after.Size())
	}
}

func TestFileStoreFailsClosedOnChecksumCorruption(t *testing.T) {
	root := t.TempDir()
	codec := testJSONCodec{id: "test-json-v1"}
	store, err := NewFileStore(root, codec)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(context.Background(), testValidatedEvent(t, "ses_corrupt", "evt_1", "one")); err != nil {
		t.Fatal(err)
	}
	path := store.sessionPath("ses_corrupt")
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		t.Fatal(err)
	}
	if info.Size() < 1 {
		file.Close()
		t.Fatal("durable log unexpectedly empty")
	}
	if _, err := file.Seek(-1, 2); err != nil {
		file.Close()
		t.Fatal(err)
	}
	var last [1]byte
	if _, err := file.Read(last[:]); err != nil {
		file.Close()
		t.Fatal(err)
	}
	last[0] ^= 0xff
	if _, err := file.Seek(-1, 2); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if _, err := file.Write(last[:]); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewFileStore(root, codec)
	if err != nil {
		t.Fatal(err)
	}
	_, err = reopened.ListSession(context.Background(), "ses_corrupt")
	if !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("checksum corruption did not fail closed: %v", err)
	}
}

func TestFileStoreAppendIsIdempotentByEventID(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root, testJSONCodec{id: "test-json-v1"})
	if err != nil {
		t.Fatal(err)
	}
	event := testValidatedEvent(t, "ses_dup", "evt_1", "same")
	if err := store.Append(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	path := store.sessionPath("ses_dup")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Size() != after.Size() {
		t.Fatalf("idempotent retry appended another frame: before=%d after=%d", before.Size(), after.Size())
	}

	conflict := testValidatedEvent(t, "ses_dup", "evt_1", "different")
	if err := store.Append(context.Background(), conflict); !errors.Is(err, ErrEventIDConflict) {
		t.Fatalf("conflicting event id was not rejected: %v", err)
	}
}

func TestFileStoreDeleteSessionPhysicallyRemovesLog(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root, testJSONCodec{id: "test-json-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(context.Background(), testValidatedEvent(t, "ses_delete", "evt_1", "one")); err != nil {
		t.Fatal(err)
	}
	path := store.sessionPath("ses_delete")
	if strings.Contains(filepath.Base(path), "ses_delete") {
		t.Fatalf("session id leaked into durable filename: %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSession(context.Background(), "ses_delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session log still exists after deletion: %v", err)
	}
	events, err := store.ListSession(context.Background(), "ses_delete")
	if err != nil || len(events) != 0 {
		t.Fatalf("deleted session remained readable: events=%v err=%v", events, err)
	}
}

func TestFileStoreRejectsCodecMismatch(t *testing.T) {
	root := t.TempDir()
	first, err := NewFileStore(root, testJSONCodec{id: "codec-a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Append(context.Background(), testValidatedEvent(t, "ses_codec", "evt_1", "one")); err != nil {
		t.Fatal(err)
	}
	second, err := NewFileStore(root, testJSONCodec{id: "codec-b"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = second.ListSession(context.Background(), "ses_codec")
	if !errors.Is(err, ErrCodecMismatch) {
		t.Fatalf("codec mismatch was not rejected: %v", err)
	}
}

func TestFileStoreUsesPrivateUnixModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits are not a Windows access-control guarantee")
	}
	root := filepath.Join(t.TempDir(), "private")
	store, err := NewFileStore(root, testJSONCodec{id: "test-json-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(context.Background(), testValidatedEvent(t, "ses_mode", "evt_1", "one")); err != nil {
		t.Fatal(err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if rootInfo.Mode().Perm() != 0o700 {
		t.Fatalf("unexpected store directory mode: %o", rootInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(store.sessionPath("ses_mode"))
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("unexpected session log mode: %o", fileInfo.Mode().Perm())
	}
}

func testValidatedEvent(t *testing.T, sessionID, eventID, reason string) observation.ValidatedEvent {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"schema_version":  "1.0",
		"event_id":        eventID,
		"session_id":      sessionID,
		"recording_epoch": 1,
		"event_type":      "content_observation",
		"wall_time":       "2026-09-16T00:00:00Z",
		"monotonic_ms":    1,
		"capture_mode":    "metadata_only",
		"payload":         map[string]any{"reason": reason},
	})
	if err != nil {
		t.Fatal(err)
	}
	event, err := observation.DecodeAndValidate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return event
}
