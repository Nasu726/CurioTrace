package store

import (
	"bytes"
	"context"
	"testing"
)

func TestSystemKeyProviderIntegratesWithEncryptedFileStoreAcrossReopen(t *testing.T) {
	secretStore := newFakeSecretStore()
	provider := deterministicSystemProvider(t, secretStore)
	codec, err := NewAESGCMCodec(provider)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	first, err := NewFileStore(root, codec)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	event := testValidatedEvent(t, "ses_system_provider", "evt_1", "system-provider-secret")
	if err := first.Append(context.Background(), event); err != nil {
		t.Fatal(err)
	}

	// Recreate the provider and codec against the same simulated OS secret store.
	// A different entropy stream proves reopening uses persisted key material and
	// does not silently generate a replacement key.
	secondProvider, err := newSystemKeyProvider(secretStore, bytes.NewReader(bytes.Repeat([]byte{0xee}, 256)))
	if err != nil {
		t.Fatal(err)
	}
	secondCodec, err := NewAESGCMCodec(secondProvider)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFileStore(root, secondCodec)
	if err != nil {
		t.Fatal(err)
	}
	events, err := second.ListSession(context.Background(), "ses_system_provider")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID() != "evt_1" {
		t.Fatalf("encrypted session was not recoverable after provider recreation: %#v", events)
	}
}
