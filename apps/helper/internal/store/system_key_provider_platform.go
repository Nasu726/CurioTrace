package store

import (
	"errors"
	"fmt"
)

var ErrSecureStoreUnavailable = errors.New("required OS secure secret store unavailable")

// NewSystemKeyProvider opens the production OS-backed secret store for the
// current platform and wires it to CurioTrace's encrypted-storage key lifecycle.
// It must never fall back to a plaintext/application-file secret store.
func NewSystemKeyProvider() (*SystemKeyProvider, error) {
	secretStore, err := newPlatformSecureSecretStore()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSecureStoreUnavailable, err)
	}
	provider, err := newSystemKeyProvider(secretStore, nil)
	if err != nil {
		return nil, err
	}
	return provider, nil
}
