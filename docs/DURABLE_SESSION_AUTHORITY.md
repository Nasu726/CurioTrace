# Durable helper session authority

Status: M1 engineering contract. Product lifecycle/privacy semantics remain governed by `docs/PRODUCT_SPEC.md` and `docs/DECISIONS.md`.

## Purpose

CurioTrace must never silently resume content capture after a helper crash or OS restart. The helper therefore persists its authoritative session control state independently of browser memory and independently of the observation-event log.

The durable snapshot contains only:

```text
state
session_id
recording_epoch
schema version
```

It contains no URL, page title, DOM text, OCR text, screenshot, selection/copy content, or other browsing observation payload.

## Startup rule

Opening helper authority follows this sequence:

```text
load latest durable snapshot
  -> validate framing/checksum/schema/state semantics
  -> if RECORDING or PAUSED:
       increment recording_epoch
       change state to INTERRUPTED
       durably append INTERRUPTED snapshot
  -> only after that durable append succeeds, expose helper authority
```

A persisted `RECORDING` or `PAUSED` state is evidence of an unfinished previous helper lifetime, not authorization to continue recording.

If the startup interruption cannot be durably committed, helper authority initialization fails. CurioTrace must not expose the stale recording state or pretend recovery succeeded.

An already `INTERRUPTED` snapshot remains `INTERRUPTED` across later helper restarts without repeatedly increasing its epoch. `FINISHED` is likewise preserved until a new explicit Start creates another session.

## Transition durability

Transitions that can grant capture authority are commit-before-expose:

- `Start`: `RECORDING` becomes visible only after its snapshot is durably saved.
- `Resume`: `RECORDING` becomes visible only after its new epoch/state is durably saved.

For transitions out of `RECORDING`, fail-closed capture revocation has priority over durable bookkeeping:

- a successful Pause/Stop persists the new non-recording state before acknowledging it;
- if that persistence fails while the current state is `RECORDING`, in-memory authority is immediately changed to `INTERRUPTED` with a fresh epoch and a best-effort attempt is made to persist that safer state;
- explicit/internal Interrupt revokes in-memory capture authority before attempting persistence.

This asymmetry is intentional. Failure to persist a transition must never be used as a reason to keep known capture authority alive.

## Observation-store failure

If a validated observation cannot be durably appended, the helper changes current authority to `INTERRUPTED` and invalidates the epoch before responding with the storage error. The extension must treat the returned non-recording state as authoritative and discard in-flight old-epoch results.

## State journal format

The M1 baseline is a small append-only journal in the application-private helper state directory.

- fixed format magic/version;
- bounded JSON snapshot payload;
- frame length;
- CRC32C for accidental corruption detection;
- synchronous file flush before transition acknowledgement;
- `0700` directory / `0600` managed file permissions on POSIX;
- partial trailing writes from a crash may be truncated back to the last complete frame;
- checksum failure, unknown JSON fields, unsupported versions, malformed state, or other non-tail corruption fails closed rather than being repaired silently.

CRC32C is not a cryptographic integrity mechanism. This journal stores only random session identifiers and control metadata. Browsing observations remain in the separately authenticated/encrypted durable observation store.

The journal is bounded. If it reaches its engineering size limit, state persistence fails closed rather than growing without limit. Future compaction may replace the implementation without changing the lifecycle contract.

## Epoch invariants

- every transition into or out of active recording advances the epoch;
- startup conversion from persisted `RECORDING`/`PAUSED` to `INTERRUPTED` advances the epoch;
- old epoch observations are rejected even when the session ID matches;
- `INTERRUPTED` state itself never authorizes observations;
- epoch exhaustion is treated as an error; CurioTrace does not wrap a 64-bit epoch.

## Protocol consequences

The helper advertises `durable_session_authority_v1` once this implementation is present.

A control transition rejected specifically because its authority state could not be durably committed returns:

```text
accepted = false
reason = STATE_PERSISTENCE_ERROR
state = <current helper-authoritative safe state>
session_id = <current session or null>
recording_epoch = <current epoch>
```

The extension must not interpret an unsuccessful control round trip optimistically. When uncertainty exists, local capture remains disabled until a fresh helper handshake establishes authority.

## Deliberately separate work

This layer does not choose:

- the final application-private root-directory resolver/installer;
- the concrete Windows/macOS/Linux secret-store adapter used by encrypted observation records;
- cross-process multi-helper coordination;
- long-term session-history indexing.

M1 assumes one authoritative helper process for a profile. Launching multiple helpers against the same profile requires a future explicit locking/coordination design; it must not be enabled accidentally.
