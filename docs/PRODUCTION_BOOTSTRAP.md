# Production helper bootstrap

Status: M1 production composition contract. The composition root is implemented, but the shipped Native Messaging entrypoint is not switched to it until the remaining platform prerequisite is complete.

## Purpose

The authoritative native helper needs one composition path that wires the already-tested M1 pieces together consistently:

```text
platform-local managed paths
  -> exclusive profile ownership
  -> production KeyProvider
  -> AES-256-GCM RecordCodec
  -> per-session FileStore
  -> durable session-authority repository
  -> helper Authority
  -> Native Messaging Handler
```

`apps/helper/internal/bootstrap.Open` is that composition root.

## Fail-closed opening order

The bootstrap:

1. requires an explicit `KeyProvider`;
2. resolves or accepts the platform-local CurioTrace paths;
3. validates/creates the managed filesystem layout;
4. acquires the exclusive OS-backed profile lock;
5. builds the AES-256-GCM codec and FileStore;
6. checks encrypted-store/key readiness;
7. only then opens durable helper session authority;
8. constructs the protocol Handler from that authority and store.

If profile ownership, the key/store path, or authority persistence is unavailable, no authoritative helper runtime is returned. Any failure after lock acquisition releases the lock before returning.

The ordering is deliberate: a second helper is rejected before it can touch the production key provider or apply authority recovery.

See `docs/PROFILE_LOCK.md` for the ownership contract.

## Restart and shutdown semantics

Opening the authoritative session authority preserves the existing product rule:

- durable unfinished `RECORDING` / `PAUSED`
- becomes durable `INTERRUPTED`
- with a fresh recording epoch
- before the opened runtime is exposed.

On graceful helper shutdown, `Runtime.Close` first transitions active `RECORDING` / `PAUSED` authority to durable `INTERRUPTED`, then releases the kernel profile lock. On crash/forced termination, the OS releases the kernel lock and the next helper performs the startup recovery above.

The bootstrap integration tests verify this together with encrypted observation reopen and exclusive profile ownership.

## CLI boundary

This bootstrap is **not** a general-purpose CLI data reader.

`session.OpenAuthority` has intentional restart side effects. If a second CLI process opened the same authority journal while a live helper was recording, it could misinterpret that persisted state as a helper restart and interrupt the live session.

Therefore:

- the authoritative helper may use this bootstrap;
- the CLI must not use it merely to inspect data;
- production CLI inspection uses the separate read-only observation-store path;
- read-only inspection never opens/mutates helper authority;
- future CLI commands that mutate shared profile state must define explicit coordination with the live helper.

## Why the Native Messaging main is not switched yet

Single-profile/helper process coordination is now implemented in the bootstrap. One production prerequisite remains before the shipped Native Messaging entrypoint can use it:

1. a concrete supported OS secret-store adapter for the production `SystemKeyProvider`.

Until that exists, `curiotrace-helper` continues to fail closed rather than silently using a testing key/provider.

## Testing

Current tests cover:

- missing KeyProvider rejection;
- complete runtime composition;
- exclusive helper ownership;
- second-helper rejection before key-provider access;
- lock release after failed bootstrap;
- graceful active-authority interruption before unlock;
- encrypted durable event persistence and reopen;
- unfinished Recording -> Interrupted recovery on reopen;
- secure-store/key readiness failure before authority open;
- readiness failure after an already-open runtime loses key-store access;
- Linux lock behavior in CI;
- Windows/macOS profile-lock and bootstrap cross-compilation in CI.
