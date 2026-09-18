# Read-only session inspection

Status: M1 CLI/storage contract.

## Purpose

CLI-first CurioTrace needs to inspect encrypted durable observations without becoming a second helper authority or mutating a live profile.

The inspection path is intentionally distinct from the authoritative helper bootstrap.

## Invariants

Read-only inspection:

- opens only the observation log required for the requested session;
- uses read-only file descriptors;
- never appends, truncates, repairs, chmods, or creates observation files/directories;
- never opens or mutates the helper authority journal;
- never calls `KeyProvider.CurrentKey` merely to inspect existing data;
- resolves historical record keys only through record key IDs / `KeyByID`;
- rejects symlinked observation roots and symlinked session-log entries;
- validates codec binding, CRC, authenticated decryption, event validation, session identity, and duplicate-event consistency;
- treats an incomplete trailing frame as a retryable read failure rather than repairing it.

The last point matters when a live helper is concurrently appending. The CLI must not take ownership of crash recovery merely because it observed a transient partial frame.

## Implementation

- `store.ReadOnlyFileStore` implements the non-mutating log reader.
- `internal/inspection.Open` composes platform paths + AES-GCM decoding + the read-only reader.
- `internal/inspection.Open` does not call `platformpath.Ensure`.
- `internal/inspection.Open` does not call `session.OpenAuthority`.

The production CLI still requires a native OS-backed KeyProvider adapter before this reader can be wired by default.

## Relationship to authoritative helper

The authoritative helper may repair a known incomplete trailing durable frame under the FileStore crash-recovery contract.

The CLI may not.

A CLI process must also never infer helper restart from the authority journal. Opening helper authority is reserved for the authoritative helper process and has intentional `RECORDING/PAUSED -> INTERRUPTED` startup semantics.

## Current limitation

The per-session filename is a hash of the session ID. Therefore this path supports inspection when the session ID is known, but it does not yet provide a session catalog. A later catalog/list command must be designed without weakening encrypted-storage/privacy semantics or scanning/decrypting more content than necessary.
