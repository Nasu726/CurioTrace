package store

import "context"

const AES256KeyBytes = 32

// KeyMaterial is caller-owned secret material returned by a KeyProvider.
// Providers must return a fresh key slice that the caller may overwrite after
// constructing cryptographic state. Key IDs are non-secret rotation handles.
type KeyMaterial struct {
	ID  string
	Key []byte
}

// KeyProvider isolates platform key storage from durable-log semantics.
//
// CurrentKey returns the key used for new records. KeyByID returns historical
// key material needed to read an existing record after key rotation.
type KeyProvider interface {
	CurrentKey(ctx context.Context) (KeyMaterial, error)
	KeyByID(ctx context.Context, keyID string) (KeyMaterial, error)
}
