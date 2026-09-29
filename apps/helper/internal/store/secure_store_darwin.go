//go:build darwin && cgo

package store

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

static CFStringRef ct_cfstring(const char *value) {
	return CFStringCreateWithCString(kCFAllocatorDefault, value, kCFStringEncodingUTF8);
}

static int ct_status_item_not_found(OSStatus status) {
	return status == errSecItemNotFound;
}

static void ct_secret_free(unsigned char *value, size_t length) {
	if (value == NULL) {
		return;
	}
	volatile unsigned char *cursor = value;
	for (size_t i = 0; i < length; i++) {
		cursor[i] = 0;
	}
	free(value);
}

static OSStatus ct_keychain_get(
	const char *service,
	const char *account,
	unsigned char **out,
	size_t *out_length
) {
	*out = NULL;
	*out_length = 0;

	CFStringRef service_ref = ct_cfstring(service);
	CFStringRef account_ref = ct_cfstring(account);
	if (service_ref == NULL || account_ref == NULL) {
		if (service_ref != NULL) CFRelease(service_ref);
		if (account_ref != NULL) CFRelease(account_ref);
		return errSecAllocate;
	}

	const void *keys[] = {
		kSecClass,
		kSecAttrService,
		kSecAttrAccount,
		kSecReturnData,
		kSecMatchLimit,
	};
	const void *values[] = {
		kSecClassGenericPassword,
		service_ref,
		account_ref,
		kCFBooleanTrue,
		kSecMatchLimitOne,
	};
	CFDictionaryRef query = CFDictionaryCreate(
		kCFAllocatorDefault,
		keys,
		values,
		5,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	CFRelease(service_ref);
	CFRelease(account_ref);
	if (query == NULL) {
		return errSecAllocate;
	}

	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(query, &result);
	CFRelease(query);
	if (status != errSecSuccess) {
		if (result != NULL) CFRelease(result);
		return status;
	}
	if (result == NULL || CFGetTypeID(result) != CFDataGetTypeID()) {
		if (result != NULL) CFRelease(result);
		return errSecDecode;
	}

	CFDataRef data = (CFDataRef)result;
	CFIndex length = CFDataGetLength(data);
	if (length < 0) {
		CFRelease(result);
		return errSecDecode;
	}
	if (length > 0) {
		const UInt8 *bytes = CFDataGetBytePtr(data);
		if (bytes == NULL) {
			CFRelease(result);
			return errSecDecode;
		}
		unsigned char *copy = (unsigned char *)malloc((size_t)length);
		if (copy == NULL) {
			CFRelease(result);
			return errSecAllocate;
		}
		memcpy(copy, bytes, (size_t)length);
		*out = copy;
	}
	*out_length = (size_t)length;
	CFRelease(result);
	return errSecSuccess;
}

static OSStatus ct_keychain_set(
	const char *service,
	const char *account,
	const unsigned char *value,
	size_t value_length
) {
	CFStringRef service_ref = ct_cfstring(service);
	CFStringRef account_ref = ct_cfstring(account);
	if (service_ref == NULL || account_ref == NULL) {
		if (service_ref != NULL) CFRelease(service_ref);
		if (account_ref != NULL) CFRelease(account_ref);
		return errSecAllocate;
	}

	CFDataRef data_ref = CFDataCreate(kCFAllocatorDefault, value, (CFIndex)value_length);
	if (data_ref == NULL) {
		CFRelease(service_ref);
		CFRelease(account_ref);
		return errSecAllocate;
	}

	const void *query_keys[] = {kSecClass, kSecAttrService, kSecAttrAccount};
	const void *query_values[] = {kSecClassGenericPassword, service_ref, account_ref};
	CFDictionaryRef query = CFDictionaryCreate(
		kCFAllocatorDefault,
		query_keys,
		query_values,
		3,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	if (query == NULL) {
		CFRelease(data_ref);
		CFRelease(service_ref);
		CFRelease(account_ref);
		return errSecAllocate;
	}

	const void *update_keys[] = {kSecValueData};
	const void *update_values[] = {data_ref};
	CFDictionaryRef update = CFDictionaryCreate(
		kCFAllocatorDefault,
		update_keys,
		update_values,
		1,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	if (update == NULL) {
		CFRelease(query);
		CFRelease(data_ref);
		CFRelease(service_ref);
		CFRelease(account_ref);
		return errSecAllocate;
	}

	OSStatus status = SecItemUpdate(query, update);
	CFRelease(update);
	if (status == errSecItemNotFound) {
		const void *add_keys[] = {kSecClass, kSecAttrService, kSecAttrAccount, kSecValueData};
		const void *add_values[] = {kSecClassGenericPassword, service_ref, account_ref, data_ref};
		CFDictionaryRef add = CFDictionaryCreate(
			kCFAllocatorDefault,
			add_keys,
			add_values,
			4,
			&kCFTypeDictionaryKeyCallBacks,
			&kCFTypeDictionaryValueCallBacks
		);
		if (add == NULL) {
			status = errSecAllocate;
		} else {
			status = SecItemAdd(add, NULL);
			CFRelease(add);
		}
	}

	CFRelease(query);
	CFRelease(data_ref);
	CFRelease(service_ref);
	CFRelease(account_ref);
	return status;
}

static OSStatus ct_keychain_delete(const char *service, const char *account) {
	CFStringRef service_ref = ct_cfstring(service);
	CFStringRef account_ref = ct_cfstring(account);
	if (service_ref == NULL || account_ref == NULL) {
		if (service_ref != NULL) CFRelease(service_ref);
		if (account_ref != NULL) CFRelease(account_ref);
		return errSecAllocate;
	}

	const void *keys[] = {kSecClass, kSecAttrService, kSecAttrAccount};
	const void *values[] = {kSecClassGenericPassword, service_ref, account_ref};
	CFDictionaryRef query = CFDictionaryCreate(
		kCFAllocatorDefault,
		keys,
		values,
		3,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	CFRelease(service_ref);
	CFRelease(account_ref);
	if (query == NULL) {
		return errSecAllocate;
	}

	OSStatus status = SecItemDelete(query);
	CFRelease(query);
	return status;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"
)

const (
	macOSProductionKeychainService = "CurioTrace.Storage"
	macOSMaxSecretBytes            = 4096
)

type macOSKeychainStore struct {
	service string
}

var _ secureSecretStore = (*macOSKeychainStore)(nil)

func newPlatformSecureSecretStore() (secureSecretStore, error) {
	return newMacOSKeychainStore(macOSProductionKeychainService)
}

func newMacOSKeychainStore(service string) (*macOSKeychainStore, error) {
	service = strings.TrimSpace(service)
	if service == "" || strings.ContainsRune(service, '\x00') {
		return nil, ErrInvalidSecretName
	}
	return &macOSKeychainStore{service: service}, nil
}

func (s *macOSKeychainStore) Get(key string) ([]byte, error) {
	if err := validateMacOSKeychainKey(key); err != nil {
		return nil, err
	}
	service := C.CString(s.service)
	account := C.CString(key)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))

	var value *C.uchar
	var valueLength C.size_t
	status := C.ct_keychain_get(service, account, &value, &valueLength)
	if value != nil {
		defer C.ct_secret_free(value, valueLength)
	}
	if C.ct_status_item_not_found(status) != 0 {
		return nil, ErrSecretNotFound
	}
	if status != 0 {
		return nil, macOSKeychainStatusError("read", status)
	}
	if uint64(valueLength) > macOSMaxSecretBytes {
		return nil, errors.New("macOS Keychain returned oversized secret")
	}
	if valueLength == 0 {
		return []byte{}, nil
	}
	return C.GoBytes(unsafe.Pointer(value), C.int(valueLength)), nil
}

func (s *macOSKeychainStore) Set(key string, value []byte) error {
	if err := validateMacOSKeychainKey(key); err != nil {
		return err
	}
	if len(value) > macOSMaxSecretBytes {
		return ErrSecretTooLarge
	}
	service := C.CString(s.service)
	account := C.CString(key)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))

	var secret unsafe.Pointer
	if len(value) != 0 {
		secret = C.CBytes(value)
		defer C.ct_secret_free((*C.uchar)(secret), C.size_t(len(value)))
	}
	status := C.ct_keychain_set(
		service,
		account,
		(*C.uchar)(secret),
		C.size_t(len(value)),
	)
	if status != 0 {
		return macOSKeychainStatusError("write", status)
	}
	return nil
}

func (s *macOSKeychainStore) Remove(key string) error {
	if err := validateMacOSKeychainKey(key); err != nil {
		return err
	}
	service := C.CString(s.service)
	account := C.CString(key)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))

	status := C.ct_keychain_delete(service, account)
	if C.ct_status_item_not_found(status) != 0 {
		return ErrSecretNotFound
	}
	if status != 0 {
		return macOSKeychainStatusError("delete", status)
	}
	return nil
}

func validateMacOSKeychainKey(key string) error {
	if key == "" || strings.ContainsRune(key, '\x00') {
		return ErrInvalidSecretName
	}
	return nil
}

func macOSKeychainStatusError(operation string, status C.OSStatus) error {
	return fmt.Errorf("macOS Keychain %s failed: OSStatus %d", operation, int32(status))
}
