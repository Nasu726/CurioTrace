//go:build darwin && !cgo

package store

import "errors"

var errMacOSKeychainRequiresCGO = errors.New("macOS Keychain adapter requires cgo")

func newPlatformSecureSecretStore() (secureSecretStore, error) {
	return nil, errMacOSKeychainRequiresCGO
}
