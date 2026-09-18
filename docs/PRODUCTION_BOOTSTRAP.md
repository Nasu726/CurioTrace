# Production helper bootstrap

Status: M1 production composition contract. The composition root is implemented, but the shipped Native Messaging entrypoint is not switched to it until the remaining platform prerequisites are complete.

## Purpose

The authoritative native helper needs one composition path that wires the already-tested M1 pieces together consistently:

```text
platform-local managed paths
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
4. builds the AES-256-GCM codec and FileStore;
5. checks encrypted-store/key readiness;
6. only then opens durable helper session authority;
7. constructs the protocol Handler from that authority and store.

If the key/store path is unavailable, no authoritative helper runtime is returned.

This ordering means a helper instance does not expose a partially initialized Recording-capable runtime merely because the authority journal exists.

## Restart semantics

Opening the authoritative session authority preserves the existing product rule:

- durable unfinished `RECORDING` / `PAUSED`
- becomes durable `INTERRUPTED`
- with a fresh recording epoch
- before the opened runtime is exposed.

The bootstrap integration test verifies this together with encrypted observation reopen.

## CLI boundary

This bootstrap is **not** a general-purpose CLI data reader.

`session.OpenAuthority` has intentional restart side effects. If a second CLI process opened the same authority journal while a live helper was recording, it could misinterpret that persisted state as a helper restart and interrupt the live session.

Therefore:

- the authoritative helper may use this bootstrap;
- the CLI must not use it merely to inspect data;
- production CLI inspection needs a separate read-only observation-store path;
- read-only inspection must never open/mutate helper authority;
- cross-process/profile coordination must be settled before commands that mutate shared profile state are enabled.

## Why the Native Messaging main is not switched yet

Two prerequisites remain:

1. a concrete supported OS secret-store adapter for the production `SystemKeyProvider`;
2. explicit single-profile/helper process coordination so multiple helper processes cannot concurrently become authoritative for the same profile.

Until those exist, `curiotrace-helper` continues to fail closed rather than silently using a testing key/provider.

## Testing

Current tests cover:

- missing KeyProvider rejection;
- complete runtime composition;
- encrypted durable event persistence and reopen;
- unfinished Recording -> Interrupted recovery on reopen;
- secure-store/key readiness failure before authority open;
- readiness failure after an already-open runtime loses key-store access.
