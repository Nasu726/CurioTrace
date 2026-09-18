package store

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type historicalOnlyKeyProvider struct {
	keyID        string
	key          []byte
	currentCalls int
}

func (p *historicalOnlyKeyProvider) CurrentKey(context.Context) (KeyMaterial, error) {
	p.currentCalls++
	return KeyMaterial{}, errors.New("CurrentKey must not be used for read-only inspection")
}

func (p *historicalOnlyKeyProvider) KeyByID(_ context.Context, keyID string) (KeyMaterial, error) {
	if keyID != p.keyID {
		return KeyMaterial{}, errTestKeyNotFound
	}
	return KeyMaterial{ID: keyID, Key: bytes.Clone(p.key)}, nil
}

func TestReadOnlyFileStoreReadsEncryptedFileStoreWithoutCurrentKey(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writerKeys := testKeys("key-readonly")
	writerCodec, err := NewAESGCMCodec(writerKeys)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := NewFileStore(root, writerCodec)
	if err != nil {
		t.Fatal(err)
	}

	event := testValidatedEvent(t, "ses_readonly", "evt_1", "visible-safe-content")
	if err := writer.Append(ctx, event); err != nil {
		t.Fatal(err)
	}

	readerKeys := &historicalOnlyKeyProvider{
		keyID: "key-readonly",
		key:   bytes.Clone(writerKeys.keys["key-readonly"]),
	}
	readerCodec, err := NewAESGCMCodec(readerKeys)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewReadOnlyFileStore(root, readerCodec)
	if err != nil {
		t.Fatal(err)
	}

	events, err := reader.ListSession(ctx, "ses_readonly")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID() != "evt_1" {
		t.Fatalf("unexpected events: %#v", events)
	}
	if readerKeys.currentCalls != 0 {
		t.Fatalf("read-only inspection called CurrentKey %d times", readerKeys.currentCalls)
	}
}

func TestReadOnlyFileStoreDoesNotRepairIncompleteTail(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	codec, err := NewAESGCMCodec(testKeys("key-tail"))
	if err != nil {
		t.Fatal(err)
	}
	writer, err := NewFileStore(root, codec)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Append(ctx, testValidatedEvent(t, "ses_tail", "evt_1", "complete")); err != nil {
		t.Fatal(err)
	}

	path := writer.sessionPath("ses_tail")
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
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	reader, err := NewReadOnlyFileStore(root, codec)
	if err != nil {
		t.Fatal(err)
	}
	events, err := reader.ListSession(ctx, "ses_tail")
	if !errors.Is(err, ErrIncompleteDurableTail) {
		t.Fatalf("expected ErrIncompleteDurableTail, events=%#v err=%v", events, err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() {
		t.Fatalf("read-only reader mutated log size: before=%d after=%d", before.Size(), after.Size())
	}
}

func TestReadOnlyFileStoreDoesNotCreateMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	codec, err := NewAESGCMCodec(testKeys("key-missing-root"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewReadOnlyFileStore(root, codec)
	if !errors.Is(err, ErrInvalidReadOnlyStore) {
		t.Fatalf("expected ErrInvalidReadOnlyStore, got %v", err)
	}
	if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("reader created missing root: %v", statErr)
	}
}

func TestReadOnlyFileStoreRejectsSymlinkRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires environment-specific privileges on Windows")
	}
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "observations-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	codec, err := NewAESGCMCodec(testKeys("key-symlink"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewReadOnlyFileStore(link, codec)
	if !errors.Is(err, ErrUnsafeReadOnlyStore) {
		t.Fatalf("expected ErrUnsafeReadOnlyStore, got %v", err)
	}
}

func TestReadOnlyFileStoreRejectsSymlinkSessionLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires environment-specific privileges on Windows")
	}
	ctx := context.Background()
	root := t.TempDir()
	codec, err := NewAESGCMCodec(testKeys("key-log-link"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewReadOnlyFileStore(root, codec)
	if err != nil {
		t.Fatal(err)
	}

	sessionID := "ses_link"
	target := filepath.Join(root, "target.ctlog")
	if err := os.WriteFile(target, []byte("not a log"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, readOnlySessionPath(root, sessionID)); err != nil {
		t.Fatal(err)
	}
	_, err = reader.ListSession(ctx, sessionID)
	if !errors.Is(err, ErrUnsafeReadOnlyStore) {
		t.Fatalf("expected ErrUnsafeReadOnlyStore, got %v", err)
	}
}

func TestReadOnlyFileStoreReturnsEmptyForMissingSession(t *testing.T) {
	root := t.TempDir()
	codec, err := NewAESGCMCodec(testKeys("key-empty"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewReadOnlyFileStore(root, codec)
	if err != nil {
		t.Fatal(err)
	}
	events, err := reader.ListSession(context.Background(), "ses_unknown")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("unexpected events: %#v", events)
	}
}
