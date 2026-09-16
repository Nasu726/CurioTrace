package store

import (
	"errors"
	"runtime"
)

type SystemSecretBackend string

const (
	SystemSecretBackendWindowsCredentialManager SystemSecretBackend = "windows-credential-manager"
	SystemSecretBackendMacOSKeychain            SystemSecretBackend = "macos-keychain"
	SystemSecretBackendLinuxSecretService       SystemSecretBackend = "linux-secret-service"
)

var ErrUnsupportedSecureStorePlatform = errors.New("no supported system secure-secret backend")

// requiredSystemSecretBackend returns exactly one accepted OS-backed secret
// store. It deliberately has no generic file/pass/keyctl fallback.
func requiredSystemSecretBackend(goos string) (SystemSecretBackend, error) {
	switch goos {
	case "windows":
		return SystemSecretBackendWindowsCredentialManager, nil
	case "darwin":
		return SystemSecretBackendMacOSKeychain, nil
	case "linux":
		return SystemSecretBackendLinuxSecretService, nil
	default:
		return "", ErrUnsupportedSecureStorePlatform
	}
}

func currentSystemSecretBackend() (SystemSecretBackend, error) {
	return requiredSystemSecretBackend(runtime.GOOS)
}
