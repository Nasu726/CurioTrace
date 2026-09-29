# CurioTrace profile ownership lock

Status: M1 helper-authority safety contract.

## Purpose

Exactly one native helper may own one CurioTrace profile as authoritative session controller at a time.

Without an exclusive profile lock, two helper processes could both open the same durable authority journal, provision/read keys, accept browser observations, or independently apply restart recovery. That would invalidate the single-authority recording model.

## Lock model

Ownership is represented by an OS kernel lock held on the profile's `authority/helper.lock` file.

The file's existence is **not** ownership. The file may remain after clean shutdown or process termination. A new helper must attempt to acquire the kernel lock and only proceed when acquisition succeeds.

Platform adapters:

- Linux: non-blocking exclusive `flock`;
- macOS: non-blocking exclusive `flock`;
- Windows: non-blocking exclusive `LockFileEx`;
- other platforms: fail closed.

The lock file is created with private process-user permissions where POSIX mode bits apply. Symlinked profile roots and symlinked/non-regular lock entries are rejected.

This is not presented as a complete defense against a hostile same-user process racing filesystem operations. It is an ownership/co-ordination boundary for normal supported operation.

## Bootstrap ordering

The authoritative production bootstrap must acquire the profile lock **before** touching the production key provider or opening durable helper authority.

Required order:

```text
resolve paths
  -> validate/create managed layout
  -> acquire exclusive profile lock
  -> check key/encrypted-store readiness
  -> open durable authority
  -> expose Handler/runtime
```

Therefore a second helper is rejected before it can provision a key or perform authority recovery.

Any bootstrap failure after lock acquisition must release the lock before returning.

## Shutdown

On graceful helper shutdown:

1. if authority is `RECORDING` or `PAUSED`, transition it to durable `INTERRUPTED` with a fresh epoch;
2. release the profile kernel lock;
3. close the lock handle.

If interruption persistence fails, in-memory capture authority is still revoked by the existing authority contract. The lock is released so a later helper can attempt normal durable recovery; if the durable state path remains unusable, that helper fails closed.

On process crash or forced termination, the OS releases the kernel lock automatically. The next helper then applies the existing startup rule that persisted unfinished `RECORDING` / `PAUSED` becomes `INTERRUPTED` before authority is exposed.

## CLI boundary

Read-only CLI inspection does not acquire this authoritative helper lock and does not open helper authority. It uses the separate read-only observation path.

Any future CLI command that mutates shared profile state must define its coordination with the live helper explicitly rather than assuming the inspection contract is sufficient.

## Tests

The M1 tests cover:

- first owner succeeds;
- second owner is rejected;
- closing the first owner permits a later owner;
- persistent lock-file existence is not mistaken for ownership;
- unsafe/symlink roots and lock entries are rejected;
- bootstrap rejects the second helper before touching its key provider;
- failed bootstrap does not leak the lock;
- graceful runtime close persists `INTERRUPTED` before releasing ownership;
- Linux tests execute in CI;
- Windows and macOS profile-lock/bootstrap packages are cross-compiled in CI.
