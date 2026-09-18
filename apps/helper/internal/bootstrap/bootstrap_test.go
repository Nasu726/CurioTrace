package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/platformpath"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/session"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/store"
)

type staticKeyProvider struct {
	id   string
	key  []byte
	err  error
}

func (p *staticKeyProvider) CurrentKey(context.Context) (store.KeyMaterial, error) {
	if p.err != nil {
		return store.KeyMaterial{}, p.err
	}
	return store.KeyMaterial{ID: p.id, Key: bytes.Clone(p.key)}, nil
}

func (p *staticKeyProvider) KeyByID(_ context.Context, keyID string) (store.KeyMaterial, error) {
	if p.err != nil {
		return store.KeyMaterial{}, p.err
	}
	if keyID != p.id {
		return store.KeyMaterial{}, errors.New("unknown key")
	}
	return store.KeyMaterial{ID: p.id, Key: bytes.Clone(p.key)}, nil
}

func testKeyProvider() *staticKeyProvider {
	return &staticKeyProvider{
		id:  "test-key-v1",
		key: bytes.Repeat([]byte{0x42}, store.AES256KeyBytes),
	}
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

func TestOpenRequiresProductionKeyProviderBeforeFilesystemBootstrap(t *testing.T) {
	paths := testPaths(t)
	_, err := Open(context.Background(), Options{Paths: &paths})
	if !errors.Is(err, ErrKeyProviderRequired) {
		t.Fatalf("expected ErrKeyProviderRequired, got %v", err)
	}
}

func TestOpenComposesEncryptedStoreDurableAuthorityAndHandler(t *testing.T) {
	paths := testPaths(t)
	runtime, err := Open(context.Background(), Options{
		Paths:       &paths,
		KeyProvider: testKeyProvider(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Handler == nil || runtime.Store == nil || runtime.Authority == nil {
		t.Fatalf("incomplete runtime: %#v", runtime)
	}
	if !runtime.Authority.IsDurable() {
		t.Fatal("authority must be durable")
	}
	if err := runtime.Ready(context.Background()); err != nil {
		t.Fatalf("runtime not ready: %v", err)
	}
	if runtime.Paths != paths {
		t.Fatalf("unexpected paths: %+v", runtime.Paths)
	}
}

func TestReopenInterruptsUnfinishedRecordingAndKeepsEncryptedEventsReadable(t *testing.T) {
	ctx := context.Background()
	paths := testPaths(t)
	keys := testKeyProvider()

	first, err := Open(ctx, Options{Paths: &paths, KeyProvider: keys})
	if err != nil {
		t.Fatal(err)
	}
	started, err := first.Authority.Start()
	if err != nil {
		t.Fatal(err)
	}
	event := mustValidated(t, `{
		"schema_version":"1.0",
		"event_id":"evt_bootstrap_1",
		"session_id":"`+started.SessionID+`",
		"recording_epoch":`+itoa(started.RecordingEpoch)+`,
		"event_type":"visibility",
		"wall_time":"2026-09-18T04:00:00Z",
		"monotonic_ms":10,
		"capture_mode":"metadata_only",
		"payload":{"reason":"tab_active"}
	}`)
	if err := first.Store.Append(ctx, event); err != nil {
		t.Fatal(err)
	}

	second, err := Open(ctx, Options{Paths: &paths, KeyProvider: keys})
	if err != nil {
		t.Fatal(err)
	}
	recovered := second.Authority.Snapshot()
	if recovered.State != session.Interrupted {
		t.Fatalf("state=%s want=%s", recovered.State, session.Interrupted)
	}
	if recovered.SessionID != started.SessionID {
		t.Fatalf("session changed: %q != %q", recovered.SessionID, started.SessionID)
	}
	if recovered.RecordingEpoch <= started.RecordingEpoch {
		t.Fatalf("epoch did not advance: start=%d recovered=%d", started.RecordingEpoch, recovered.RecordingEpoch)
	}
	events, err := second.Store.ListSession(ctx, started.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID() != "evt_bootstrap_1" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestKeyReadinessFailurePreventsAuthorityOpen(t *testing.T) {
	paths := testPaths(t)
	keys := testKeyProvider()
	keys.err = errors.New("secure store locked")

	runtime, err := Open(context.Background(), Options{Paths: &paths, KeyProvider: keys})
	if runtime != nil {
		t.Fatalf("unexpected runtime: %#v", runtime)
	}
	if !errors.Is(err, ErrBootstrapNotReady) {
		t.Fatalf("expected ErrBootstrapNotReady, got %v", err)
	}
}

func TestReadyReflectsLaterKeyStoreFailure(t *testing.T) {
	paths := testPaths(t)
	keys := testKeyProvider()
	runtime, err := Open(context.Background(), Options{Paths: &paths, KeyProvider: keys})
	if err != nil {
		t.Fatal(err)
	}
	keys.err = errors.New("secure store unavailable")
	if err := runtime.Ready(context.Background()); !errors.Is(err, ErrBootstrapNotReady) {
		t.Fatalf("expected bootstrap readiness failure, got %v", err)
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

func itoa(value uint64) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
