package inspection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/platformpath"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/store"
)

type fixedKeyProvider struct {
	id            string
	key           []byte
	currentCalls  int
	forbidCurrent bool
}

func (p *fixedKeyProvider) CurrentKey(context.Context) (store.KeyMaterial, error) {
	p.currentCalls++
	if p.forbidCurrent {
		return store.KeyMaterial{}, errors.New("CurrentKey forbidden in inspection")
	}
	return store.KeyMaterial{ID: p.id, Key: bytes.Clone(p.key)}, nil
}

func (p *fixedKeyProvider) KeyByID(_ context.Context, keyID string) (store.KeyMaterial, error) {
	if keyID != p.id {
		return store.KeyMaterial{}, errors.New("unknown historical key")
	}
	return store.KeyMaterial{ID: p.id, Key: bytes.Clone(p.key)}, nil
}

func testPaths(t *testing.T) platformpath.Paths {
	t.Helper()
	root := filepath.Join(t.TempDir(), "curiotrace")
	return platformpath.Paths{
		Root:         root,
		Observations: filepath.Join(root, "observations"),
		Authority:    filepath.Join(root, "authority"),
	}
}

func TestOpenRequiresKeyProviderBeforeTouchingProfile(t *testing.T) {
	paths := testPaths(t)
	_, err := Open(context.Background(), Options{Paths: &paths})
	if !errors.Is(err, ErrKeyProviderRequired) {
		t.Fatalf("expected ErrKeyProviderRequired, got %v", err)
	}
	if _, statErr := os.Stat(paths.Root); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("inspection touched profile root: %v", statErr)
	}
}

func TestOpenDoesNotCreateMissingObservationRoot(t *testing.T) {
	paths := testPaths(t)
	keys := &fixedKeyProvider{
		id:  "key-inspection",
		key: bytes.Repeat([]byte{0x31}, store.AES256KeyBytes),
	}
	_, err := Open(context.Background(), Options{Paths: &paths, KeyProvider: keys})
	if !errors.Is(err, ErrInspectionNotReady) {
		t.Fatalf("expected ErrInspectionNotReady, got %v", err)
	}
	if _, statErr := os.Stat(paths.Observations); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("inspection created observations root: %v", statErr)
	}
	if keys.currentCalls != 0 {
		t.Fatalf("inspection called CurrentKey %d times", keys.currentCalls)
	}
}

func TestInspectionReadsEncryptedEventsWithoutOpeningAuthorityOrCurrentKey(t *testing.T) {
	ctx := context.Background()
	paths := testPaths(t)
	if err := platformpath.Ensure(paths); err != nil {
		t.Fatal(err)
	}

	authorityMarker := []byte("authority-must-remain-untouched")
	authorityPath := filepath.Join(paths.Authority, "authority.state")
	if err := os.WriteFile(authorityPath, authorityMarker, 0o600); err != nil {
		t.Fatal(err)
	}

	key := bytes.Repeat([]byte{0x52}, store.AES256KeyBytes)
	writerKeys := &fixedKeyProvider{id: "key-existing", key: key}
	writerCodec, err := store.NewAESGCMCodec(writerKeys)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := store.NewFileStore(paths.Observations, writerCodec)
	if err != nil {
		t.Fatal(err)
	}

	sessionID := "ses_0123456789abcdef0123456789abcdef"
	event := mustValidated(t, `{
		"schema_version":"1.0",
		"event_id":"evt_inspect_1",
		"session_id":"`+sessionID+`",
		"recording_epoch":4,
		"event_type":"navigation",
		"wall_time":"2026-09-18T04:40:00Z",
		"monotonic_ms":12,
		"capture_mode":"metadata_only",
		"source":{"url":"https://example.test/"},
		"payload":{"reason":"navigation"}
	}`)
	if err := writer.Append(ctx, event); err != nil {
		t.Fatal(err)
	}

	readerKeys := &fixedKeyProvider{
		id:            "key-existing",
		key:           key,
		forbidCurrent: true,
	}
	runtime, err := Open(ctx, Options{Paths: &paths, KeyProvider: readerKeys})
	if err != nil {
		t.Fatal(err)
	}
	if readerKeys.currentCalls != 0 {
		t.Fatalf("Open called CurrentKey %d times", readerKeys.currentCalls)
	}
	events, err := runtime.Reader.ListSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID() != "evt_inspect_1" {
		t.Fatalf("unexpected events: %#v", events)
	}
	if readerKeys.currentCalls != 0 {
		t.Fatalf("read called CurrentKey %d times", readerKeys.currentCalls)
	}

	after, err := os.ReadFile(authorityPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, authorityMarker) {
		t.Fatalf("inspection mutated authority file: %q", after)
	}
}

func TestCancelledInspectionDoesNotTouchProfile(t *testing.T) {
	paths := testPaths(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	keys := &fixedKeyProvider{
		id:  "key-cancelled",
		key: bytes.Repeat([]byte{0x12}, store.AES256KeyBytes),
	}
	_, err := Open(ctx, Options{Paths: &paths, KeyProvider: keys})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if _, statErr := os.Stat(paths.Root); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cancelled inspection touched profile root: %v", statErr)
	}
}

func mustValidated(t *testing.T, raw string) observation.ValidatedEvent {
	t.Helper()
	event, err := observation.DecodeAndValidate(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return event
}
