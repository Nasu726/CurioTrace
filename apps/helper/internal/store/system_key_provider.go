package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
)

const (
	systemCurrentKeyPointer = "storage-current-key-id"
	systemKeyPrefix         = "storage-key/"
	systemKeyIDBytes        = 16
)

var (
	ErrSecretNotFound       = errors.New("secure secret not found")
	ErrSystemKeyState       = errors.New("invalid system key state")
	ErrSystemKeyProvision   = errors.New("system key provisioning failed")
	ErrInvalidSystemKeyID   = errors.New("invalid system key id")
	ErrSecureStoreRequired  = errors.New("secure secret store required")
)

// secureSecretStore is the narrow boundary implemented by OS-specific secure
// credential-store adapters. Implementations must not silently fall back to a
// plaintext/application file store.
type secureSecretStore interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Remove(key string) error
}

// SystemKeyProvider owns CurioTrace's encrypted-storage keys while delegating
// secret persistence to an OS secure credential store.
//
// It intentionally stores only a non-secret current-key pointer outside the key
// item itself. Historical key entries remain addressable by ID for record
// decryption after rotation.
type SystemKeyProvider struct {
	mu     sync.Mutex
	store  secureSecretStore
	random io.Reader
}

func newSystemKeyProvider(store secureSecretStore, random io.Reader) (*SystemKeyProvider, error) {
	if store == nil {
		return nil, ErrSecureStoreRequired
	}
	if random == nil {
		random = rand.Reader
	}
	return &SystemKeyProvider{store: store, random: random}, nil
}

func (p *SystemKeyProvider) CurrentKey(ctx context.Context) (KeyMaterial, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return KeyMaterial{}, err
	}

	keyID, err := p.loadCurrentKeyID()
	if errors.Is(err, ErrSecretNotFound) {
		return p.provisionLocked(ctx)
	}
	if err != nil {
		return KeyMaterial{}, fmt.Errorf("read current system key pointer: %w", err)
	}
	return p.loadKeyLocked(ctx, keyID)
}

func (p *SystemKeyProvider) KeyByID(ctx context.Context, keyID string) (KeyMaterial, error) {
	if err := validateSystemKeyID(keyID); err != nil {
		return KeyMaterial{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loadKeyLocked(ctx, keyID)
}

// Rotate creates and activates a new key while retaining historical key items.
// Rotation is explicit; CurioTrace never rotates by deleting a key still needed
// to decrypt retained session records.
func (p *SystemKeyProvider) Rotate(ctx context.Context) (KeyMaterial, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return KeyMaterial{}, err
	}
	return p.provisionLocked(ctx)
}

func (p *SystemKeyProvider) loadCurrentKeyID() (string, error) {
	data, err := p.store.Get(systemCurrentKeyPointer)
	if err != nil {
		return "", err
	}
	defer clear(data)
	keyID := string(data)
	if err := validateSystemKeyID(keyID); err != nil {
		return "", fmt.Errorf("%w: current key pointer: %v", ErrSystemKeyState, err)
	}
	return keyID, nil
}

func (p *SystemKeyProvider) loadKeyLocked(ctx context.Context, keyID string) (KeyMaterial, error) {
	if err := contextError(ctx); err != nil {
		return KeyMaterial{}, err
	}
	data, err := p.store.Get(systemKeyPrefix + keyID)
	if err != nil {
		if errors.Is(err, ErrSecretNotFound) {
			return KeyMaterial{}, fmt.Errorf("%w: key %q referenced but missing", ErrSystemKeyState, keyID)
		}
		return KeyMaterial{}, fmt.Errorf("read system key %q: %w", keyID, err)
	}
	defer clear(data)
	if err := contextError(ctx); err != nil {
		return KeyMaterial{}, err
	}
	if len(data) != AES256KeyBytes {
		return KeyMaterial{}, fmt.Errorf("%w: key %q has invalid length", ErrSystemKeyState, keyID)
	}
	return KeyMaterial{ID: keyID, Key: bytes.Clone(data)}, nil
}

func (p *SystemKeyProvider) provisionLocked(ctx context.Context) (KeyMaterial, error) {
	if err := contextError(ctx); err != nil {
		return KeyMaterial{}, err
	}

	idBytes := make([]byte, systemKeyIDBytes)
	if _, err := io.ReadFull(p.random, idBytes); err != nil {
		return KeyMaterial{}, fmt.Errorf("%w: generate key id: %w", ErrSystemKeyProvision, err)
	}
	keyID := hex.EncodeToString(idBytes)
	clear(idBytes)

	key := make([]byte, AES256KeyBytes)
	if _, err := io.ReadFull(p.random, key); err != nil {
		clear(key)
		return KeyMaterial{}, fmt.Errorf("%w: generate encryption key: %w", ErrSystemKeyProvision, err)
	}
	defer clear(key)

	if err := contextError(ctx); err != nil {
		return KeyMaterial{}, err
	}
	keyName := systemKeyPrefix + keyID
	if err := p.store.Set(keyName, key); err != nil {
		return KeyMaterial{}, fmt.Errorf("%w: store encryption key: %w", ErrSystemKeyProvision, err)
	}

	if err := contextError(ctx); err != nil {
		cleanupErr := p.removeProvisionedKey(keyName)
		return KeyMaterial{}, errors.Join(err, cleanupErr)
	}
	if err := p.store.Set(systemCurrentKeyPointer, []byte(keyID)); err != nil {
		cleanupErr := p.removeProvisionedKey(keyName)
		return KeyMaterial{}, errors.Join(
			fmt.Errorf("%w: activate encryption key: %w", ErrSystemKeyProvision, err),
			cleanupErr,
		)
	}

	return KeyMaterial{ID: keyID, Key: bytes.Clone(key)}, nil
}

func (p *SystemKeyProvider) removeProvisionedKey(keyName string) error {
	if err := p.store.Remove(keyName); err != nil && !errors.Is(err, ErrSecretNotFound) {
		return fmt.Errorf("cleanup unactivated encryption key: %w", err)
	}
	return nil
}

func validateSystemKeyID(keyID string) error {
	if len(keyID) != systemKeyIDBytes*2 {
		return ErrInvalidSystemKeyID
	}
	decoded, err := hex.DecodeString(keyID)
	if err != nil || len(decoded) != systemKeyIDBytes || hex.EncodeToString(decoded) != keyID {
		return ErrInvalidSystemKeyID
	}
	return nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
