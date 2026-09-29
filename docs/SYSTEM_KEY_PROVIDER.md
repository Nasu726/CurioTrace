# System key-provider boundary

Status: M1 engineering contract. The key-lifecycle core and Windows Credential Manager adapter are implemented; macOS Keychain and Linux Secret Service adapters remain pending.

## Purpose

CurioTrace's durable session log is encrypted with per-record AES-256-GCM. The encryption codec needs a stable current key for new records and historical keys for old records after rotation. Raw encryption keys must not be stored beside session logs or in an application plaintext file.

`SystemKeyProvider` implements that lifecycle while delegating actual secret persistence to a narrow `secureSecretStore` adapter. `NewSystemKeyProvider()` is the production constructor and must resolve only the required OS-native backend for the current platform.

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

## Windows Credential Manager adapter

The Windows production adapter is implemented directly against the Win32 Credential Manager APIs rather than through a generic cross-platform keyring fallback:

- `CredWriteW` stores/replaces a generic credential;
- `CredReadW` retrieves it;
- `CredDeleteW` removes it;
- `CredFree` releases Windows-owned read buffers.

CurioTrace uses `CRED_TYPE_GENERIC` and `CRED_PERSIST_LOCAL_MACHINE` under a fixed `CurioTrace/Storage/...` target namespace. The target and fixed username metadata contain no browsing content or encryption key material.

A retrieved `CredentialBlob` is copied into caller-owned Go memory before `CredFree` is called. The adapter rejects invalid secret names and values larger than the Windows generic credential-blob bound used by CurioTrace. Win32 failures are returned without including secret values in diagnostics.

Windows CI runs the real adapter on a Windows runner using a randomized test namespace. The integration test covers missing/read/write/overwrite/delete semantics, caller-owned returned slices, first key provisioning, provider reopen, rotation, and historical-key lookup. Production namespace credentials are not used by tests.

## Concurrency

The M1 provider serializes provisioning and rotation within one helper process. Cross-process authority is enforced separately by the OS-backed profile ownership lock in `docs/PROFILE_LOCK.md`. The authoritative helper acquires profile ownership before the production key provider is touched, so two helper processes cannot race first-use provisioning for the same profile under normal supported operation.

Read-only CLI inspection does not become helper authority and uses the separate read-only observation path.

## Cancellation limitation

`KeyProvider` accepts `context.Context` and the core avoids touching the secure store when a request is already canceled. Native desktop credential-store calls may be blocking and do not necessarily expose context-aware cancellation. CurioTrace must not fake cancellation by spawning potentially leaked goroutines around such calls. The adapter boundary remains replaceable if packaging or platform API requirements change.

## Adapter status

Current status:

- Windows Credential Manager: **implemented and exercised on a Windows CI runner**;
- macOS Keychain: pending;
- Linux Secret Service: pending.

Earlier library investigation remains relevant to the remaining adapters:

- `zalando/go-keyring` has useful cross-platform coverage but its macOS path uses `/usr/bin/security`; an upstream security concern argues that this weakens expected Keychain access-control semantics, so it is not selected for CurioTrace.
- `99designs/keyring` exposes an explicit `AllowedBackends` whitelist and its macOS backend uses Keychain APIs directly. However, its latest release/repository activity is old enough that CurioTrace treats it only as a replaceable adapter candidate rather than a product-semantic dependency.
- a direct macOS Keychain adapter may require cgo/Security.framework integration. A production macOS build must treat codesigning/keychain behavior as a platform packaging requirement rather than pretending the helper is universally static.

The shipped Native Messaging entrypoint remains fail-closed until the supported desktop OS adapters required by the product are implemented and wired through the production bootstrap. Windows support alone does not activate a partial production recording mode.
