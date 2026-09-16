# System key-provider boundary

Status: M1 engineering contract. This document describes key lifecycle semantics independently of the eventual OS credential-store adapter.

## Purpose

CurioTrace's durable session log is encrypted with per-record AES-256-GCM. The encryption codec needs a stable current key for new records and historical keys for old records after rotation. Raw encryption keys must not be stored beside session logs or in an application plaintext file.

`SystemKeyProvider` implements that lifecycle while delegating actual secret persistence to a narrow `secureSecretStore` adapter.

## Provisioning

On first use, when no current-key pointer exists:

1. generate a 128-bit random non-secret key ID;
2. generate a 256-bit random AES key;
3. persist the key item in the secure OS store;
4. only after key persistence succeeds, persist/activate the current-key ID pointer;
5. return a caller-owned copy of the key.

The ordering is deliberate. A failed pointer write may leave an unactivated key that can be cleaned up; it must not leave a pointer to a key that was never safely stored.

If the current-key pointer exists but is malformed, references a missing key, or references key material with an invalid size, the provider fails closed. It does **not** silently generate a replacement because doing so could make retained encrypted sessions permanently unreadable while masking the key-loss event.

## Rotation

Rotation is explicit. Before rotating, the provider verifies that the existing current pointer and current key are internally consistent. It then provisions a new key and moves the current pointer to the new ID.

Historical keys are retained because encrypted records contain their non-secret key ID and still require those keys for decryption. Deleting a historical key is therefore a destructive retention operation and is not part of ordinary rotation.

## Secure-store policy

CurioTrace accepts exactly one OS-native secure credential-store family per supported desktop OS:

- Windows: Credential Manager;
- macOS: Keychain;
- Linux: Secret Service.

There is deliberately no automatic fallback to application files, encrypted-file keyrings, `pass`, Linux `keyctl`, or another backend merely because the preferred service is unavailable. If the required secure store cannot be opened or unlocked, encrypted-store readiness fails and Start/Resume remain non-recording.

The OS adapter must map a missing item to `ErrSecretNotFound`, return caller-owned byte slices, synchronously persist/copy values passed to `Set`, and never log secret bytes.

## Concurrency

The M1 provider serializes provisioning and rotation within one helper process. The product architecture assumes one authoritative native helper. Cross-process locking is not claimed by this core. If packaging later permits multiple concurrent helpers, a process-level singleton/lock must be added before relying on first-use provisioning semantics across processes.

## Cancellation limitation

`KeyProvider` accepts `context.Context` and the core avoids touching the secure store when a request is already canceled. Some third-party desktop-keyring libraries expose blocking OS calls without context-aware cancellation. CurioTrace must not fake cancellation by spawning potentially leaked goroutines around such calls. Any selected OS adapter must document this limitation, and the adapter boundary remains replaceable if direct platform APIs become necessary.

## Adapter status

The lifecycle core is dependency-independent. A production OS adapter is still pending.

Library investigation found:

- `zalando/go-keyring` has useful cross-platform coverage but its macOS path uses `/usr/bin/security`; an open upstream security issue argues that this weakens expected Keychain access-control semantics, so it is not selected for CurioTrace.
- `99designs/keyring` exposes an explicit `AllowedBackends` whitelist and its macOS backend uses Keychain APIs directly. However, its latest release and repository commit are from 2022, so it is considered only as a replaceable pinned adapter, not a product-semantic dependency. If used, CurioTrace must whitelist exactly one native backend for the current OS and disable all generic fallback behavior.
- the macOS backend of `99designs/keyring` requires cgo. A production macOS build must therefore treat cgo/codesigning/keychain behavior as a platform packaging requirement rather than pretending the helper is universally static.

The repository must not claim OS key-store integration complete until the actual adapter and platform-specific build/integration tests exist.
