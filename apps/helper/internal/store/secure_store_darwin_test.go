//go:build darwin && cgo

package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestMacOSKeychainStoreRoundTripOverwriteAndRemove(t *testing.T) {
	secretStore := newMacOSTestStore(t)
	const key = "round-trip"
	t.Cleanup(func() { _ = secretStore.Remove(key) })

	if _, err := secretStore.Get(key); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("missing Get error=%v want ErrSecretNotFound", err)
	}

	first := []byte("first-secret-value")
	if err := secretStore.Set(key, first); err != nil {
		t.Fatal(err)
	}
	got, err := secretStore.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, first) {
		t.Fatalf("Get=%q want=%q", got, first)
	}

	got[0] ^= 0xff
	again, err := secretStore.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, first) {
		t.Fatalf("mutating caller-owned result changed stored value: %q", again)
	}

	second := []byte("replacement-secret")
	if err := secretStore.Set(key, second); err != nil {
		t.Fatal(err)
	}
	got, err = secretStore.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, second) {
		t.Fatalf("overwritten Get=%q want=%q", got, second)
	}

	if err := secretStore.Remove(key); err != nil {
		t.Fatal(err)
	}
	if _, err := secretStore.Get(key); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("Get after Remove error=%v want ErrSecretNotFound", err)
	}
	if err := secretStore.Remove(key); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("second Remove error=%v want ErrSecretNotFound", err)
	}
}

func TestMacOSKeychainStoreSupportsEmptyValue(t *testing.T) {
	secretStore := newMacOSTestStore(t)
	const key = "empty-value"
	t.Cleanup(func() { _ = secretStore.Remove(key) })

	if err := secretStore.Set(key, nil); err != nil {
		t.Fatal(err)
	}
	got, err := secretStore.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("empty secret length=%d", len(got))
	}
}

func TestMacOSKeychainStoreRejectsInvalidInputs(t *testing.T) {
	if _, err := newMacOSKeychainStore(""); !errors.Is(err, ErrInvalidSecretName) {
		t.Fatalf("empty service error=%v", err)
	}
	secretStore := newMacOSTestStore(t)
	if err := secretStore.Set("", []byte("x")); !errors.Is(err, ErrInvalidSecretName) {
		t.Fatalf("empty key error=%v", err)
	}
	if err := secretStore.Set("bad\x00key", []byte("x")); !errors.Is(err, ErrInvalidSecretName) {
		t.Fatalf("NUL key error=%v", err)
	}
	if err := secretStore.Set("too-large", make([]byte, macOSMaxSecretBytes+1)); !errors.Is(err, ErrSecretTooLarge) {
		t.Fatalf("large value error=%v", err)
	}
}

func TestMacOSSystemKeyProviderPersistsAndRotatesInKeychain(t *testing.T) {
	ctx := context.Background()
	secretStore := newMacOSTestStore(t)

	firstID := strings.Repeat("11", systemKeyIDBytes)
	secondID := strings.Repeat("33", systemKeyIDBytes)
	t.Cleanup(func() {
		_ = secretStore.Remove(systemCurrentKeyPointer)
		_ = secretStore.Remove(systemKeyPrefix + firstID)
		_ = secretStore.Remove(systemKeyPrefix + secondID)
	})

	randomBytes := make([]byte, 0, 2*(systemKeyIDBytes+AES256KeyBytes))
	randomBytes = append(randomBytes, bytes.Repeat([]byte{0x11}, systemKeyIDBytes)...)
	randomBytes = append(randomBytes, bytes.Repeat([]byte{0x22}, AES256KeyBytes)...)
	randomBytes = append(randomBytes, bytes.Repeat([]byte{0x33}, systemKeyIDBytes)...)
	randomBytes = append(randomBytes, bytes.Repeat([]byte{0x44}, AES256KeyBytes)...)

	firstProvider, err := newSystemKeyProvider(secretStore, bytes.NewReader(randomBytes))
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstProvider.CurrentKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(first.Key)
	if first.ID != firstID || !bytes.Equal(first.Key, bytes.Repeat([]byte{0x22}, AES256KeyBytes)) {
		t.Fatalf("unexpected first key: id=%q len=%d", first.ID, len(first.Key))
	}

	secondProvider, err := newSystemKeyProvider(secretStore, bytes.NewReader(randomBytes[systemKeyIDBytes+AES256KeyBytes:]))
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := secondProvider.CurrentKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ID != first.ID || !bytes.Equal(reopened.Key, first.Key) {
		clear(reopened.Key)
		t.Fatalf("reopened key mismatch: id=%q", reopened.ID)
	}
	clear(reopened.Key)

	rotated, err := secondProvider.Rotate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(rotated.Key)
	if rotated.ID != secondID || !bytes.Equal(rotated.Key, bytes.Repeat([]byte{0x44}, AES256KeyBytes)) {
		t.Fatalf("unexpected rotated key: id=%q len=%d", rotated.ID, len(rotated.Key))
	}

	historical, err := secondProvider.KeyByID(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(historical.Key, first.Key) {
		clear(historical.Key)
		t.Fatal("historical key changed after rotation")
	}
	clear(historical.Key)

	current, err := secondProvider.CurrentKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != secondID || !bytes.Equal(current.Key, rotated.Key) {
		clear(current.Key)
		t.Fatalf("current key after rotation mismatch: id=%q", current.ID)
	}
	clear(current.Key)
}

func newMacOSTestStore(t *testing.T) *macOSKeychainStore {
	t.Helper()
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	secretStore, err := newMacOSKeychainStore("CurioTrace.Test." + hex.EncodeToString(suffix[:]))
	if err != nil {
		t.Fatal(err)
	}
	return secretStore
}
