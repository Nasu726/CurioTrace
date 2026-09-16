# CurioTrace Durable Storage

Status: replaceable M1 engineering baseline. Product privacy/retention guarantees remain defined by `docs/PRODUCT_SPEC.md` and `docs/DECISIONS.md`.

## 1. Why the M1 durable unit is a session log

CurioTrace's durable source unit is one browsing session. Cross-session search, Wiki maintenance, and long-term knowledge integration are explicitly outside CurioTrace's responsibility.

For M1, a per-session append-only log fits the workload better than introducing a relational database immediately:

- observation events are naturally ordered by session time;
- M2 exposure reconstruction consumes a session event stream;
- M3 inspection reads one session;
- M5 exports one session;
- deleting a session maps to deleting one managed event-log file;
- no database server/runtime is required;
- the Go helper remains a small self-contained executable.

This is an implementation baseline, not a promise never to migrate to SQLite or another store if later requirements justify indexing/querying across larger data sets.

## 2. File identity and layout

`FileStore` stores one `.ctlog` file per session under an application-private root directory.

The filename is derived from SHA-256 of the session ID rather than containing the session ID itself. This avoids leaking raw session identifiers through filenames and prevents path traversal through session identifiers.

On POSIX-like systems the implementation requests:

- store directory: `0700`;
- session file: `0600`.

These permissions are ordinary OS access controls, not a defense against a compromised user account/OS.

## 3. Framing and integrity

Each log begins with:

- fixed CurioTrace log magic/version;
- the `RecordCodec` identifier.

Each event frame contains:

1. encoded-record byte length;
2. CRC32C of the encoded record;
3. encoded record bytes.

The encoded record is bounded in size.

CRC32C exists to detect accidental corruption and torn/corrupted frames. It is **not cryptographic authentication** and must never be described as a confidentiality or tamper-resistance boundary.

A future authenticated-encryption codec provides cryptographic integrity/confidentiality for record bytes.

## 4. Crash recovery

Append behavior is:

```text
ValidatedEvent
  -> RecordCodec.Encode
  -> length + CRC32C frame
  -> append
  -> file Sync
```

On reopen, the store scans from the header through complete validated frames.

Only one recovery case is repaired automatically:

- an incomplete final frame at EOF, consistent with a process/OS interruption while appending.

That trailing partial frame is truncated back to the last complete record and synced.

The store fails closed instead of silently repairing when it encounters:

- invalid file magic;
- incompatible codec ID;
- impossible/oversized record length;
- checksum mismatch;
- record decode/validation failure;
- a record for a different session;
- one event ID reused for different event content.

This distinction prevents arbitrary corruption from being mislabeled as a harmless crash tail.

## 5. Idempotency

Native Messaging/control recovery may cause a caller to retry an observation.

Within a session:

- retrying the same `event_id` with identical validated event content is a no-op;
- reusing the same `event_id` for different event content fails closed.

The index is rebuilt from the durable file after helper restart, so this property does not depend solely on process memory.

## 6. Validation boundary

`FileStore` implements the existing `Store` interface, whose append method accepts `observation.ValidatedEvent`, not raw protocol bytes.

The durable path therefore remains:

```text
Native message
  -> session/epoch authority
  -> observation schema/privacy validation
  -> ValidatedEvent
  -> RecordCodec
  -> durable session log
```

Rejected observations are not written to the durable file merely for debugging.

## 7. Confidentiality and key management

`FileStore` deliberately does **not** define encryption or key storage. Those concerns live behind `RecordCodec` / future `KeyProvider` adapters so platform key-management choices do not contaminate session/storage semantics.

Production safety rule:

> CurioTrace must not wire a plaintext/testing codec into normal recording merely to make the file store usable.

Until a production-safe authenticated-encryption codec and key source are configured, the default helper remains unable to start a durable recording session.

A testing codec may encode validated JSON directly, but it must remain test/development-only and must not become the production default.

## 8. Session deletion

Deleting a session removes its managed `.ctlog` file and in-memory index.

This is CurioTrace's physical deletion at the application-storage level: the application no longer retains the managed session record.

Do **not** describe `os.Remove` as forensic secure erasure. Filesystem snapshots, SSD wear leveling, backups, or a compromised OS may retain recoverable historical blocks outside CurioTrace's control.

## 9. Deliberately separate follow-up work

This M1 storage layer does not yet solve:

- production authenticated encryption;
- OS-specific key/secret storage;
- helper-authoritative session-state persistence;
- restart conversion of unfinished `RECORDING` / `PAUSED` sessions to `INTERRUPTED` with a new epoch;
- application-private root-directory discovery/installation per OS;
- backup/export UX;
- format migration tooling.

Those layers should reuse the tested session-log semantics rather than weakening them.
