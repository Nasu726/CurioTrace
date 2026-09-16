package store

import (
	"errors"
	"testing"
)

func TestRequiredSystemSecretBackendHasExactlyOneNativeChoice(t *testing.T) {
	tests := map[string]SystemSecretBackend{
		"windows": SystemSecretBackendWindowsCredentialManager,
		"darwin":  SystemSecretBackendMacOSKeychain,
		"linux":   SystemSecretBackendLinuxSecretService,
	}
	for goos, want := range tests {
		t.Run(goos, func(t *testing.T) {
			got, err := requiredSystemSecretBackend(goos)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("backend mismatch: got=%q want=%q", got, want)
			}
		})
	}
}

func TestRequiredSystemSecretBackendRejectsGenericFallbackPlatforms(t *testing.T) {
	for _, goos := range []string{"freebsd", "openbsd", "android", "js", "plan9", ""} {
		backend, err := requiredSystemSecretBackend(goos)
		if !errors.Is(err, ErrUnsupportedSecureStorePlatform) || backend != "" {
			t.Fatalf("unsupported platform %q unexpectedly selected backend=%q err=%v", goos, backend, err)
		}
	}
}

func TestSystemSecretBackendPolicyContainsNoFileLikeFallback(t *testing.T) {
	for _, goos := range []string{"windows", "darwin", "linux"} {
		backend, err := requiredSystemSecretBackend(goos)
		if err != nil {
			t.Fatal(err)
		}
		switch backend {
		case SystemSecretBackendWindowsCredentialManager, SystemSecretBackendMacOSKeychain, SystemSecretBackendLinuxSecretService:
			// Allowed.
		default:
			t.Fatalf("platform %q selected non-native fallback %q", goos, backend)
		}
	}
}
