package store

import (
	"context"
	"errors"
	"testing"
)

func TestAESGCMCodecReadyRequiresCurrentKey(t *testing.T) {
	provider := &testKeyProvider{current: "missing", keys: map[string][]byte{}}
	codec, err := NewAESGCMCodec(provider)
	if err != nil {
		t.Fatal(err)
	}
	if err := codec.Ready(context.Background()); !errors.Is(err, errTestKeyNotFound) {
		t.Fatalf("missing current key did not fail readiness: %v", err)
	}
}

func TestEncryptedFileStoreReadyRejectsInvalidCurrentKey(t *testing.T) {
	provider := &testKeyProvider{
		current: "short",
		keys: map[string][]byte{
			"short": []byte("short"),
		},
	}
	codec, err := NewAESGCMCodec(provider)
	if err != nil {
		t.Fatal(err)
	}
	fileStore, err := NewFileStore(t.TempDir(), codec)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileStore.Ready(context.Background()); !errors.Is(err, ErrInvalidKeyMaterial) {
		t.Fatalf("invalid current key did not fail store readiness: %v", err)
	}
}
