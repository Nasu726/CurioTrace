//go:build !windows && !darwin

package store

import "fmt"

func newPlatformSecureSecretStore() (secureSecretStore, error) {
	backend, err := currentSystemSecretBackend()
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%s adapter is not implemented", backend)
}
