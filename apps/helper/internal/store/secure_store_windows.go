//go:build windows

package store

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	windowsCredentialTypeGeneric       = 1
	windowsCredentialPersistLocalMachine = 2
	windowsErrorNotFound               = syscall.Errno(1168)
	windowsMaxCredentialBlobBytes      = 2560
	windowsProductionCredentialPrefix  = "CurioTrace/Storage"
	windowsCredentialUserName          = "CurioTrace"
)

var (
	windowsAdvapi32       = syscall.NewLazyDLL("advapi32.dll")
	windowsCredWriteW     = windowsAdvapi32.NewProc("CredWriteW")
	windowsCredReadW      = windowsAdvapi32.NewProc("CredReadW")
	windowsCredDeleteW    = windowsAdvapi32.NewProc("CredDeleteW")
	windowsCredFree       = windowsAdvapi32.NewProc("CredFree")
	ErrInvalidSecretName = errors.New("invalid secure secret name")
	ErrSecretTooLarge     = errors.New("secure secret value too large")
)

type windowsCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type windowsCredentialStore struct {
	prefix string
}

var _ secureSecretStore = (*windowsCredentialStore)(nil)

func newPlatformSecureSecretStore() (secureSecretStore, error) {
	return newWindowsCredentialStore(windowsProductionCredentialPrefix)
}

func newWindowsCredentialStore(prefix string) (*windowsCredentialStore, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || strings.ContainsRune(prefix, '\x00') {
		return nil, ErrInvalidSecretName
	}
	return &windowsCredentialStore{prefix: strings.TrimSuffix(prefix, "/")}, nil
}

func (s *windowsCredentialStore) Get(key string) ([]byte, error) {
	target, err := s.targetName(key)
	if err != nil {
		return nil, err
	}
	targetPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return nil, ErrInvalidSecretName
	}

	var credential *windowsCredential
	result, _, callErr := windowsCredReadW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		uintptr(windowsCredentialTypeGeneric),
		0,
		uintptr(unsafe.Pointer(&credential)),
	)
	if result == 0 {
		if errors.Is(callErr, windowsErrorNotFound) {
			return nil, ErrSecretNotFound
		}
		return nil, windowsCredentialError("read credential", callErr)
	}
	if credential == nil {
		return nil, errors.New("read credential returned no credential")
	}
	defer windowsCredFree.Call(uintptr(unsafe.Pointer(credential)))

	if credential.CredentialBlobSize == 0 {
		return []byte{}, nil
	}
	if credential.CredentialBlob == nil || credential.CredentialBlobSize > windowsMaxCredentialBlobBytes {
		return nil, errors.New("read credential returned invalid secret size")
	}
	blob := unsafe.Slice(credential.CredentialBlob, int(credential.CredentialBlobSize))
	return bytes.Clone(blob), nil
}

func (s *windowsCredentialStore) Set(key string, value []byte) error {
	target, err := s.targetName(key)
	if err != nil {
		return err
	}
	if len(value) > windowsMaxCredentialBlobBytes {
		return ErrSecretTooLarge
	}

	targetPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return ErrInvalidSecretName
	}
	userPtr, err := syscall.UTF16PtrFromString(windowsCredentialUserName)
	if err != nil {
		return ErrInvalidSecretName
	}

	credential := windowsCredential{
		Type:               windowsCredentialTypeGeneric,
		TargetName:         targetPtr,
		CredentialBlobSize: uint32(len(value)),
		Persist:            windowsCredentialPersistLocalMachine,
		UserName:           userPtr,
	}
	if len(value) != 0 {
		credential.CredentialBlob = &value[0]
	}

	result, _, callErr := windowsCredWriteW.Call(
		uintptr(unsafe.Pointer(&credential)),
		0,
	)
	if result == 0 {
		return windowsCredentialError("write credential", callErr)
	}
	return nil
}

func (s *windowsCredentialStore) Remove(key string) error {
	target, err := s.targetName(key)
	if err != nil {
		return err
	}
	targetPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return ErrInvalidSecretName
	}

	result, _, callErr := windowsCredDeleteW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		uintptr(windowsCredentialTypeGeneric),
		0,
	)
	if result == 0 {
		if errors.Is(callErr, windowsErrorNotFound) {
			return ErrSecretNotFound
		}
		return windowsCredentialError("delete credential", callErr)
	}
	return nil
}

func (s *windowsCredentialStore) targetName(key string) (string, error) {
	if key == "" || strings.ContainsRune(key, '\x00') {
		return "", ErrInvalidSecretName
	}
	return s.prefix + "/" + key, nil
}

func windowsCredentialError(operation string, err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return fmt.Errorf("%s failed", operation)
	}
	return fmt.Errorf("%s failed: %w", operation, err)
}
