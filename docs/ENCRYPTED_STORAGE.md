# Encrypted durable-record boundary

Status: M1 engineering contract. Product/privacy semantics remain governed by `docs/PRODUCT_SPEC.md` and `docs/DECISIONS.md`.

## Scope

CurioTrace durable observation files may contain sensitive browsing text. The production record codec therefore encrypts each validated observation independently before it reaches the session-log frame layer.

M1 baseline:

- AES-256-GCM authenticated encryption;
- a fresh random nonce for every record;
- non-secret key IDs stored with records for key rotation;
- record header authenticated as AEAD additional data;
- decrypted bytes are independently revalidated as an observation before becoming a `ValidatedEvent` again;
- tampering, wrong keys, unavailable historical keys, malformed records, and invalid decrypted observations fail closed;
- plaintext validated-event JSON and caller-owned key slices are cleared after use where Go permits.

The implementation does **not** claim that Go runtime memory can be perfectly zeroized, nor that application-level deletion securely erases filesystem/SSD/cloud-backup remnants.

## Key-provider boundary

`KeyProvider` is deliberately separate from `FileStore` and `AESGCMCodec`.

A production provider must:

- return 32-byte AES-256 keys;
- return a fresh caller-owned key slice;
- provide the current key for new records;
- resolve historical keys by non-secret key ID while retained data still depends on them;
- fail closed when secure key storage is unavailable;
- use platform-appropriate secret storage rather than writing raw keys beside the session logs.

OS-specific key-storage adapters are a separate implementation step. Until a production provider is configured, normal recording must remain unavailable rather than falling back to plaintext durable storage.

## Start readiness gate

A configured store is not sufficient evidence that recording can safely begin. Before helper authority enters `RECORDING`, the helper calls the durable store readiness path:

```text
session.start
  -> Store.Ready
  -> FileStore.Ready
  -> RecordCodec.Ready
  -> KeyProvider.CurrentKey
  -> validate current AES-256 key material
```

Any failure rejects Start with the helper remaining `IDLE`. This prevents a known-unavailable key service or malformed current key from creating a session that can capture observations but cannot persist them.

Readiness is a preflight, not a promise that I/O can never fail later. Mid-session storage/key failures must still fail closed and are handled as runtime persistence failures rather than silently falling back to plaintext.

## Rotation

Rotation changes the `CurrentKey` result. Existing records retain their original key IDs and remain decryptable through `KeyByID`. Removing a historical key is therefore a destructive retention action for records encrypted under that key and must not happen accidentally.

The file-store codec identifier remains stable across ordinary key rotations; changing the encrypted-record wire format requires a new codec identifier/version.

## Integrity layers

The session-log CRC32C detects accidental frame corruption and helps crash recovery. It is not a security mechanism. AES-GCM authentication is the security boundary for encrypted record integrity and authenticity.
