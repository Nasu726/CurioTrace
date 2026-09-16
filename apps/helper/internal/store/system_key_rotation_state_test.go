package store

import (
	"context"
	"errors"
	"testing"
)

func TestSystemKeyProviderRotationRefusesDanglingCurrentPointer(t *testing.T) {
	secretStore := newFakeSecretStore()
	secretStore.items[systemCurrentKeyPointer] = []byte("00112233445566778899aabbccddeeff")
	provider := deterministicSystemProvider(t, secretStore)

	_, err := provider.Rotate(context.Background())
	if !errors.Is(err, ErrSystemKeyState) {
		t.Fatalf("rotation overwrote corrupt current-key state: %v", err)
	}
	if got := string(secretStore.items[systemCurrentKeyPointer]); got != "00112233445566778899aabbccddeeff" {
		t.Fatalf("failed rotation mutated current pointer: %q", got)
	}
	if indexOfPrefix(secretStore.snapshotOps(), "set:"+systemKeyPrefix) >= 0 {
		t.Fatalf("failed rotation provisioned a replacement key: %v", secretStore.snapshotOps())
	}
}
