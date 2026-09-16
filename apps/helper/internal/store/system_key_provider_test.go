package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

type fakeSecretStore struct {
	mu        sync.Mutex
	items     map[string][]byte
	ops       []string
	getErr    map[string]error
	setErr    map[string]error
	removeErr map[string]error
}

func newFakeSecretStore() *fakeSecretStore {
	return &fakeSecretStore{
		items:     make(map[string][]byte),
		getErr:    make(map[string]error),
		setErr:    make(map[string]error),
		removeErr: make(map[string]error),
	}
}

func (s *fakeSecretStore) Get(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops = append(s.ops, "get:"+key)
	if err := s.getErr[key]; err != nil {
		return nil, err
	}
	value, ok := s.items[key]
	if !ok {
		return nil, ErrSecretNotFound
	}
	return bytes.Clone(value), nil
}

func (s *fakeSecretStore) Set(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops = append(s.ops, "set:"+key)
	if err := s.setErr[key]; err != nil {
		return err
	}
	s.items[key] = bytes.Clone(value)
	return nil
}

func (s *fakeSecretStore) Remove(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops = append(s.ops, "remove:"+key)
	if err := s.removeErr[key]; err != nil {
		return err
	}
	if _, ok := s.items[key]; !ok {
		return ErrSecretNotFound
	}
	delete(s.items, key)
	return nil
}

func (s *fakeSecretStore) snapshotOps() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ops...)
}

func TestSystemKeyProviderProvisionsKeyBeforeCurrentPointer(t *testing.T) {
	secretStore := newFakeSecretStore()
	provider := deterministicSystemProvider(t, secretStore)

	material, err := provider.CurrentKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSystemKeyID(material.ID); err != nil || len(material.Key) != AES256KeyBytes {
		t.Fatalf("invalid provisioned key material: id=%q len=%d err=%v", material.ID, len(material.Key), err)
	}

	ops := secretStore.snapshotOps()
	keySet := indexOfPrefix(ops, "set:"+systemKeyPrefix)
	pointerSet := indexOf(ops, "set:"+systemCurrentKeyPointer)
	if keySet < 0 || pointerSet < 0 || keySet >= pointerSet {
		t.Fatalf("key was not persisted before activation pointer: %v", ops)
	}
	if got := string(secretStore.items[systemCurrentKeyPointer]); got != material.ID {
		t.Fatalf("unexpected active key pointer: got=%q want=%q", got, material.ID)
	}
	if got := secretStore.items[systemKeyPrefix+material.ID]; !bytes.Equal(got, material.Key) {
		t.Fatal("stored key differs from returned key")
	}
}

func TestSystemKeyProviderReturnsFreshKeyCopies(t *testing.T) {
	secretStore := newFakeSecretStore()
	provider := deterministicSystemProvider(t, secretStore)
	first, err := provider.CurrentKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	original := bytes.Clone(first.Key)
	first.Key[0] ^= 0xff

	second, err := provider.CurrentKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second.Key, original) {
		t.Fatal("caller mutation changed securely stored key")
	}
	if &first.Key[0] == &second.Key[0] {
		t.Fatal("provider reused caller-owned key slice")
	}
}

func TestSystemKeyProviderRejectsDanglingCurrentPointer(t *testing.T) {
	secretStore := newFakeSecretStore()
	secretStore.items[systemCurrentKeyPointer] = []byte("00112233445566778899aabbccddeeff")
	provider := deterministicSystemProvider(t, secretStore)

	_, err := provider.CurrentKey(context.Background())
	if !errors.Is(err, ErrSystemKeyState) {
		t.Fatalf("dangling current pointer did not fail closed: %v", err)
	}
	if indexOfPrefix(secretStore.snapshotOps(), "set:"+systemKeyPrefix) >= 0 {
		t.Fatal("provider silently replaced a missing referenced key")
	}
}

func TestSystemKeyProviderRejectsMalformedCurrentPointer(t *testing.T) {
	secretStore := newFakeSecretStore()
	secretStore.items[systemCurrentKeyPointer] = []byte("not-a-valid-key-id")
	provider := deterministicSystemProvider(t, secretStore)

	_, err := provider.CurrentKey(context.Background())
	if !errors.Is(err, ErrSystemKeyState) {
		t.Fatalf("malformed current pointer did not fail closed: %v", err)
	}
}

func TestSystemKeyProviderPointerActivationFailureLeavesNoActiveKey(t *testing.T) {
	secretStore := newFakeSecretStore()
	secretStore.setErr[systemCurrentKeyPointer] = errors.New("pointer write failed")
	provider := deterministicSystemProvider(t, secretStore)

	_, err := provider.CurrentKey(context.Background())
	if !errors.Is(err, ErrSystemKeyProvision) {
		t.Fatalf("activation failure not surfaced: %v", err)
	}
	if _, ok := secretStore.items[systemCurrentKeyPointer]; ok {
		t.Fatal("failed activation left a current-key pointer")
	}
	for key := range secretStore.items {
		if len(key) >= len(systemKeyPrefix) && key[:len(systemKeyPrefix)] == systemKeyPrefix {
			t.Fatalf("failed activation left an orphan key: %q", key)
		}
	}
}

func TestSystemKeyProviderDoesNotProvisionOnSecureStoreReadFailure(t *testing.T) {
	secretStore := newFakeSecretStore()
	secretStore.getErr[systemCurrentKeyPointer] = errors.New("secure store unavailable")
	provider := deterministicSystemProvider(t, secretStore)

	_, err := provider.CurrentKey(context.Background())
	if err == nil || errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("secure-store failure not surfaced: %v", err)
	}
	if indexOfPrefix(secretStore.snapshotOps(), "set:") >= 0 {
		t.Fatalf("provider attempted provisioning after secure-store failure: %v", secretStore.snapshotOps())
	}
}

func TestSystemKeyProviderRotationRetainsHistoricalKey(t *testing.T) {
	secretStore := newFakeSecretStore()
	provider := deterministicSystemProvider(t, secretStore)
	oldMaterial, err := provider.CurrentKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	oldKey := bytes.Clone(oldMaterial.Key)

	newMaterial, err := provider.Rotate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if newMaterial.ID == oldMaterial.ID || bytes.Equal(newMaterial.Key, oldMaterial.Key) {
		t.Fatal("rotation reused old key material")
	}
	current, err := provider.CurrentKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != newMaterial.ID || !bytes.Equal(current.Key, newMaterial.Key) {
		t.Fatal("rotation did not activate new key")
	}
	historical, err := provider.KeyByID(context.Background(), oldMaterial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(historical.Key, oldKey) {
		t.Fatal("rotation destroyed historical key")
	}
}

func TestSystemKeyProviderRejectsArbitraryHistoricalKeyNames(t *testing.T) {
	provider := deterministicSystemProvider(t, newFakeSecretStore())
	for _, keyID := range []string{"../secret", "AABBCCDDEEFF00112233445566778899", "0011"} {
		if _, err := provider.KeyByID(context.Background(), keyID); !errors.Is(err, ErrInvalidSystemKeyID) {
			t.Fatalf("invalid key id %q was accepted: %v", keyID, err)
		}
	}
}

func TestSystemKeyProviderSerializesConcurrentInitialProvisioning(t *testing.T) {
	secretStore := newFakeSecretStore()
	provider := deterministicSystemProvider(t, secretStore)

	const workers = 16
	results := make(chan KeyMaterial, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			material, err := provider.CurrentKey(context.Background())
			if err != nil {
				errs <- err
				return
			}
			results <- material
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	var expectedID string
	for material := range results {
		if expectedID == "" {
			expectedID = material.ID
		}
		if material.ID != expectedID {
			t.Fatalf("concurrent provisioning returned multiple active IDs: %q vs %q", expectedID, material.ID)
		}
	}

	keySets := 0
	for _, op := range secretStore.snapshotOps() {
		if len(op) >= len("set:"+systemKeyPrefix) && op[:len("set:"+systemKeyPrefix)] == "set:"+systemKeyPrefix {
			keySets++
		}
	}
	if keySets != 1 {
		t.Fatalf("expected one in-process key provisioning, got %d ops=%v", keySets, secretStore.snapshotOps())
	}
}

func TestSystemKeyProviderHonorsAlreadyCanceledContext(t *testing.T) {
	secretStore := newFakeSecretStore()
	provider := deterministicSystemProvider(t, secretStore)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := provider.CurrentKey(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context not honored: %v", err)
	}
	if len(secretStore.snapshotOps()) != 0 {
		t.Fatalf("canceled request touched secure store: %v", secretStore.snapshotOps())
	}
}

func deterministicSystemProvider(t *testing.T, secretStore secureSecretStore) *SystemKeyProvider {
	t.Helper()
	// Enough deterministic entropy for multiple rotations while keeping tests reproducible.
	entropy := make([]byte, 4096)
	for i := range entropy {
		entropy[i] = byte((i*31 + 17) % 251)
	}
	provider, err := newSystemKeyProvider(secretStore, bytes.NewReader(entropy))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}

func indexOfPrefix(values []string, prefix string) int {
	for i, value := range values {
		if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
			return i
		}
	}
	return -1
}

func (s *fakeSecretStore) String() string {
	return fmt.Sprintf("fakeSecretStore(%d items)", len(s.items))
}
