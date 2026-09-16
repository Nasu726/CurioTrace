# CurioTrace M1 Implementation Plan

Status: active implementation decomposition for issue #9. This plan deliberately separates browser-independent work from #8 empirical-browser decisions.

## 1. M1 outcome

M1 proves one privacy-safe end-to-end session on Chrome/Edge:

```text
explicit Start
  -> helper-authoritative recording session
  -> allowed navigation / visible-content observations
  -> validated local persistence
  -> explicit Stop
  -> inspectable machine session trace
```

M1 does **not** need robust exposure aggregation, polished Session Viewer, LLM summarization, or final Markdown generation.

## 2. Architectural layers

### A. Extension UI / permission controller

Responsibilities:

- Start/Pause/Resume/Stop controls;
- first-Start explanation and runtime host-permission request;
- refuse to enter recording when required permission is absent;
- show helper unavailable/interrupted state;
- expose current recording/exclusion state visibly enough for M1 testing.

Normative references:

- `docs/PERMISSION_ONBOARDING.md`
- `docs/PRODUCT_SPEC.md`

### B. Extension native-protocol client

Responsibilities:

- establish persistent Native Messaging connection;
- perform version/capability handshake;
- receive helper-authoritative `session_id`, state, and `recording_epoch`;
- fail closed on disconnect/protocol incompatibility;
- send only protocol-allowed data classes;
- surface backpressure/rejection reason codes without logging sensitive payloads.

Production baseline:

- `apps/extension/src/protocol-client.ts`

Independent reference oracle:

- `spikes/extension_authority_harness/protocol-client-state.mjs`

Normative reference:

- `docs/NATIVE_HELPER_PROTOCOL.md`

### C. Extension capture-authority guard

Responsibilities:

- maintain local mirror of helper capture authority;
- issue immutable capture tokens for asynchronous work;
- reject completion if connection generation/session/epoch changed;
- prevent observation materialization after Pause/Stop/Interrupted/disconnect.

Production baseline:

- `apps/extension/src/capture-authority.ts`

Independent reference oracle:

- `spikes/extension_authority_harness/capture-authority.mjs`

This logic is browser-independent and covered by CI.

### D. Browser event collector

Responsibilities:

- navigation/view activation;
- visibility/background facts;
- allowed selection/copy observations;
- content-script lifecycle while `RECORDING`;
- privacy/exclusion gate before capture work.

M1 should prefer small typed event producers over one monolithic service worker.

### E. Capture adapter

Interface conceptually returns one of:

```text
DOM_OBSERVATION
REDACTED_VISUAL_RESULT
FINGERPRINT_ONLY
METADATA_ONLY
BLOCKED
FAILED
```

M1 must not make higher layers depend on whether the observation came from DOM, visual fallback, or fingerprinting.

#### Stable now

- privacy/capture-mode meanings;
- no unredacted raster crossing extension -> helper;
- DOM-inaccessible unsafe visual semantics fail to fingerprint-only/no-content rather than OCR by default;
- async result must carry/validate capture authority token.

#### Pending #8 measurements

- exact DOM probe implementation details;
- screenshot scheduling/debounce values;
- raster-redaction performance;
- cross-origin/sandboxed iframe coverage;
- PDF/restricted-page browser behavior;
- Edge/Firefox parity quirks.

Until #8 is resolved, these are adapter internals, not assumptions in storage/schema/UI.

### F. Observation builder and validator

Extension-side builder responsibilities:

- convert browser/capture results into `observation-v1` semantics;
- attach wall-clock + monotonic time;
- attach current session/epoch;
- normalize explicit gap/error/privacy result states;
- enforce bounded payloads before transport where practical;
- never turn unknown/missing data into fabricated content.

Helper-side validation responsibilities:

- revalidate shape and privacy semantics independently of the extension;
- reject unknown/missing/null schema fields where forbidden by the durable schema;
- reject raw raster/form/clipboard-sensitive fields;
- enforce capture-mode-specific payload restrictions;
- create a `ValidatedEvent` only after validation succeeds.

Production helper baseline:

- `apps/helper/internal/observation/`

The production validator and `ValidatedEvent` boundary are implemented and CI-tested. Store APIs accept the validated wrapper instead of raw protocol data.

Normative references:

- `docs/OBSERVATION_SCHEMA.md`
- `schemas/observation-v1.schema.json`

### G. Native helper authority

Responsibilities:

- authoritative session state machine;
- epoch invalidation;
- protocol handshake;
- event validation/rejection;
- helper disconnect/recovery semantics;
- storage orchestration.

Production baseline:

- Go helper under `apps/helper/`;
- stdlib Native Messaging framing and session authority are implemented and CI-tested;
- Start and Resume fail closed when the durable store/key path is unavailable;
- mid-session durable append failure moves helper authority to `INTERRUPTED` and invalidates the epoch.

Independent reference oracle:

- `spikes/native_helper_harness/`

Implementation-stack rationale is recorded in `docs/IMPLEMENTATION_STACK.md` and remains replaceable without changing product semantics.

### H. Durable store

M1 storage contract:

- application-private local storage;
- append durable validated observations only;
- rejected payloads are not persisted for debugging;
- no raw/unredacted screenshots;
- schema version retained;
- session deletion can identify/remove all managed records for that session;
- enough metadata exists to inspect a session after browser/application restart.

Current production boundary:

- `Store.Append` accepts `ValidatedEvent`, not raw protocol payloads;
- an in-memory store exists only for tests/conformance;
- `FileStore` is the current dependency-free M1 durable-backend candidate;
- `FileStore` uses one hashed-name append-only log per session with codec binding, bounded frames, CRC32C corruption detection, `Sync`, idempotent event IDs, partial-tail crash repair, and physical managed-file deletion;
- arbitrary corruption/codec mismatch/session mismatch fails closed rather than being silently repaired;
- AES-256-GCM is the production record-confidentiality/authentication codec;
- `SystemKeyProvider` implements first-use provisioning, current-key pointers, historical-key lookup, explicit rotation, corruption detection, and fail-closed key lifecycle semantics behind a narrow OS-secret-store adapter boundary;
- Windows Credential Manager, macOS Keychain, and Linux Secret Service are the only allowed production secret-store classes; file/pass/keyctl fallback is forbidden;
- the concrete native OS adapter is still pending, so the default helper remains unable to Start normal recording.

Normative implementation notes:

- `docs/DURABLE_STORAGE.md`
- `docs/ENCRYPTED_STORAGE.md`
- `docs/SYSTEM_KEY_PROVIDER.md`

Still pending:

- concrete Windows Credential Manager / macOS Keychain / Linux Secret Service adapter;
- application-private root-directory discovery/installation per OS;
- durable helper-authoritative session state;
- restart conversion of unfinished `RECORDING` / `PAUSED` state to `INTERRUPTED` with a fresh epoch.

Do not wire a plaintext testing codec into normal recording merely to make the file backend usable.

### I. Minimal session inspector

M1 only needs a developer/basic human inspection surface showing:

- session ID/state/times;
- visited allowed source pointers;
- capture modes;
- compact semantic observations;
- blocked/failed/gap markers;
- no hidden full-page archive.

A polished Session Viewer belongs to M3.

## 3. Start flow

```text
User presses Start
  |
  +-- required broad host permission already granted? -- no --> explain rationale
  |                                                      |
  |                                                      +--> browser request denied -> remain IDLE
  |
  +-- native helper available and protocol compatible? -- no --> setup/repair; remain IDLE
  |
  +-- production-safe durable store/key path ready? ----- no --> setup/repair; remain IDLE
  |
  +-- helper session.start -> helper returns session_id + epoch + RECORDING
  |
  +-- extension capture guard applies helper authority
  |
  +-- enable/register recording-scoped collectors
  |
  `-- recording UI becomes active
```

Do not mark UI as recording until helper authority is established.

## 4. Pause/Stop flow and asynchronous races

Pause/Stop is helper-authoritative.

1. user requests Pause/Stop;
2. extension immediately enters a local `capture_requested_off` guard so it does not initiate new expensive capture work while the control round-trip is pending;
3. helper validates transition and increments epoch;
4. extension applies returned non-recording state/new epoch;
5. content observers are disabled/unregistered where practical;
6. any in-flight result holding the old capture token fails token validation and is discarded before observation construction/transport.

A control-message failure must not make the extension continue optimistically. If authority becomes uncertain, fail closed and surface Interrupted/helper-error state.

## 5. Helper disconnect flow

```text
Native port disconnects
  -> extension capture guard invalidates connection generation immediately
  -> stop initiating observations
  -> discard in-flight raw/high-fidelity material
  -> do not queue page text/screenshots for reconnect
  -> show interrupted/helper unavailable state
  -> reconnect performs fresh handshake
  -> helper decides whether continuity can be proven
  -> otherwise require explicit Resume/Stop according to product lifecycle
```

## 6. Persistence flow

Only a helper-accepted, schema/privacy-valid observation reaches durable persistence.

Required M1 layering:

```text
Native message
  -> protocol envelope validation
  -> session/epoch authority validation
  -> observation shape/schema validation
  -> privacy semantic validation
  -> ValidatedEvent
  -> AES-256-GCM RecordCodec
  -> bounded durable session-log append + Sync
  -> small acknowledgement
```

Do not write the payload to debug logs before validation. Store APIs should accept the validated type rather than raw transport data so validation cannot be accidentally skipped by ordinary call sites.

A checksum is only an accidental-corruption signal. Cryptographic confidentiality/integrity belongs to the production authenticated-encryption codec and key provider.

## 7. Testing layers

### Every CI run

Independent cheap jobs run in parallel:

- Python native-helper protocol/privacy reference tests;
- Node extension reference authority/protocol tests;
- Go production-helper format/test/build checks;
- TypeScript production-extension build/conformance tests;
- JSON schema syntax/shape validation.

The Go production-helper job includes durable-store and encrypted-key lifecycle tests for reopen, idempotency, partial-tail recovery, corruption rejection, codec mismatch, authenticated tamper rejection, key rotation, key-state corruption, session deletion, and POSIX access modes where applicable.

As production modules are added, their tests join the appropriate production job rather than replacing the independent reference oracles.

### Browser integration after #8 environment is available

- runtime permission grant/refusal;
- normal HTML capture/redaction;
- iframe cases;
- PDF/restricted surfaces;
- screenshot timing/rate;
- helper Native Messaging end-to-end;
- Pause/Stop/disconnect while screenshot/capture is in flight.

### Hard privacy tests

Synthetic canaries must never appear in forbidden durable/log/external outputs.

See `docs/EVALUATION.md`.

## 8. M1 implementation order and current status

1. **Protocol/schema/reference invariants** — completed baseline.
2. **Extension capture-authority guard** — completed reference and production-state-machine baseline.
3. **Native protocol client abstraction** — completed browser-independent production baseline.
4. **Observation validation + store interface** — completed production baseline.
5. **M1 durable event storage** — per-session framed `FileStore`, AES-256-GCM record codec, and system-key lifecycle core implemented/tested; native OS secret-store adapter and restart-state integration remain active work.
6. **Extension UI + permission flow** — wire fixed onboarding semantics.
7. **Browser event collector** — navigation/visibility first.
8. **Capture adapter** — integrate #8 findings; normal HTML first.
9. **End-to-end Chrome/Edge session** — Start -> record -> Stop -> inspect.
10. **M1 privacy/restart/adversarial verification**.

## 9. M1 stop conditions

Do not claim M1 complete if any of the following remains true:

- host permission refusal can still create a recording session;
- helper disconnect leaves capture running;
- old-epoch async results can be persisted;
- unredacted raster crosses the extension -> helper boundary on the normal semantic path;
- blocked/private/editable canaries reach durable data;
- unvalidated protocol data can reach the durable store through an ordinary production API;
- normal recording can start with a plaintext/testing durable codec or a generic secret-store fallback;
- unfinished session state can silently recover as `RECORDING` after helper/OS restart;
- browser restart/recovery semantics contradict the product lifecycle;
- #8 empirical tests contradict the selected capture adapter behavior;
- a session cannot be inspected without an LLM/network connection.

## 10. Deliberately deferred choices

M1 planning does not yet fix:

- frontend framework (if any);
- exact third-party/native OS secret-store adapter implementation;
- OCR engine mix;
- installer/update framework;
- final UI visual design;
- final capture debounce constants.

The current TypeScript extension + Go helper baseline is documented in `docs/IMPLEMENTATION_STACK.md`; changing that engineering baseline does not alter the product contract. The current per-session durable-log format is likewise an implementation baseline and may be migrated later without changing the product's observation/privacy semantics.

Deferred choices should be made from evidence/maintenance needs, not accidentally encoded into product semantics.
