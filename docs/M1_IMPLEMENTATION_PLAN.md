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

Production baseline:

- toolbar popup lifecycle controls are implemented;
- the first-Start explanation precedes the browser permission prompt;
- a denied/missing host permission cannot send helper `session.start`;
- the background rechecks host permission before Start even if the popup/controller already checked it.

Normative references:

- `docs/PERMISSION_ONBOARDING.md`
- `docs/M1_POPUP_CONTROLS.md`
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
- `apps/extension/src/native-messaging-transport.ts`
- `apps/extension/src/helper-connection.ts`

Independent reference oracle:

- `spikes/extension_authority_harness/protocol-client-state.mjs`

Normative reference:

- `docs/NATIVE_HELPER_PROTOCOL.md`
- `docs/NATIVE_MESSAGING_TRANSPORT.md`

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

Production baseline now includes the navigation/visibility layer:

- `apps/extension/src/navigation-visibility-collector.ts` produces navigation, visibility, privacy-decision, and gap observations;
- `apps/extension/src/browser-observation-event-gate.ts` attaches raw browser listeners only during helper-authorized `RECORDING` intervals;
- Pause/Stop detach browser listeners synchronously before the Native Messaging round trip and suspend local capture authority so queued work cannot cross the control boundary;
- helper disconnect/observation rejection/collector failure also detaches browser listeners;
- private/unknown-private/browser-internal/user-excluded views never persist source URL/title;
- allowed HTTP/HTTPS source URLs remove URL credentials and syntactically redact obvious credential/token/password/secret/API-key/session/OAuth parameters before persistence;
- runtime tab/window IDs remain transient implementation identifiers and are not durable observation identity;
- Start/Resume create an explicit current-active-view snapshot; Resume starts a fresh view segment after the exposure break;
- window focus loss remains an `unknown` exposure-visibility fact rather than an inference that the page became invisible.

Still pending in this layer:

- selection/copy observations;
- persistent user exclusion storage/UI;
- OS lock/suspend signal integration;
- SPA navigation detail beyond browser-observable tab URL changes.

M1 should prefer small typed event producers over one monolithic service worker.

Normative implementation note:

- `docs/M1_NAVIGATION_VISIBILITY_COLLECTOR.md`

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

Production extension baseline:

- `apps/extension/src/observation-event.ts` creates bounded identity/timing/view envelopes for the currently implemented browser event classes;
- browser-event timestamps are captured at event receipt time before asynchronous tab lookups;
- `apps/extension/src/protocol-observation-submit.ts` sends only capture-token-authorized observations and consumes helper acknowledgements;
- any rejected durable observation is terminal for the current browser-side recording path rather than silently dropping events and continuing an incomplete trace;
- a helper-authoritative rejection state such as storage failure -> `INTERRUPTED` is applied before browser cleanup/disconnect.

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
- Start and Resume fail closed when the durable observation store/key path is unavailable;
- helper-authoritative control state has a durable append-only journal independent of browsing observations;
- startup converts persisted unfinished `RECORDING`/`PAUSED` state to `INTERRUPTED` with a fresh epoch before authority is exposed;
- Start/Resume become `RECORDING` only after their authority snapshots are durably committed;
- a failed transition out of active recording revokes in-memory capture authority rather than leaving it active;
- mid-session durable observation append failure moves helper authority to `INTERRUPTED` and invalidates the epoch;
- `durable_session_authority_v1` is advertised only by helper instances actually wired to the durable authority repository.

Independent reference oracle:

- `spikes/native_helper_harness/`

Normative implementation note:

- `docs/DURABLE_SESSION_AUTHORITY.md`

Implementation-stack rationale is recorded in `docs/IMPLEMENTATION_STACK.md` and remains replaceable without changing product semantics.

### H. Durable store and platform filesystem layout

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
- helper session authority durability/restart conversion is implemented separately from observation persistence so browsing content is not copied into the control journal;
- platform-local filesystem path policy is implemented under `apps/helper/internal/platformpath`: Windows uses `LOCALAPPDATA`, macOS uses Application Support, Linux uses absolute `XDG_DATA_HOME` or `~/.local/share`; observations and authority use fixed separate child directories;
- managed final-directory symlinks/non-directories and unsafe layouts are rejected; this is not presented as complete anti-TOCTOU protection against a hostile same-user process;
- authoritative production composition is now implemented in `apps/helper/internal/bootstrap`: given a production KeyProvider it wires platform paths, AES-GCM FileStore, durable authority, and Handler;
- the concrete native OS secret-store adapter and shipped-entrypoint activation are still pending, so the default helper remains unable to Start normal recording.

Normative implementation notes:

- `docs/DURABLE_STORAGE.md`
- `docs/ENCRYPTED_STORAGE.md`
- `docs/SYSTEM_KEY_PROVIDER.md`
- `docs/DURABLE_SESSION_AUTHORITY.md`
- `docs/PLATFORM_STORAGE_PATHS.md`
- `docs/PRODUCTION_BOOTSTRAP.md`

Still pending:

- concrete Windows Credential Manager / macOS Keychain / Linux Secret Service adapter;
- switch the Native Messaging entrypoint to the authoritative production bootstrap only after its platform key provider is available;
- separate read-only encrypted observation wiring for the CLI; the CLI must not open helper authority merely to inspect data;
- explicit single-helper/profile locking or equivalent coordination before multiple helper processes could ever become authoritative for the same profile.

Do not wire a plaintext testing codec or generic secret-store fallback into normal recording merely to make the file backend usable.

### I. CLI-first session inspector

M1 needs a developer/basic human inspection surface, and the preferred baseline is CLI/text output rather than a management GUI.

It should show:

- session ID/state/times;
- visited allowed source pointers;
- capture modes;
- compact semantic observations;
- blocked/failed/gap markers;
- no hidden full-page archive.

Production CLI baseline now exists as a separate `curiotrace` binary:

- `status [--json]` reports platform paths and production readiness;
- `inspect --session <id> [--json]` renders validated durable observations through a `SessionReader` boundary;
- the default production CLI currently reports encrypted inspection unavailable because the OS secret-store/bootstrap wiring is still pending.

A polished Session Viewer is not a release requirement. Add richer GUI only when a concrete task cannot be served well by CLI/text output.

Normative implementation note:

- `docs/CLI.md`

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
  +-- platform-local managed storage root available? ---- no --> remain IDLE
  |
  +-- production-safe durable store/key path ready? ----- no --> setup/repair; remain IDLE
  |
  +-- durable session-authority journal available? ------ no --> remain IDLE
  |
  +-- helper session.start
  |     -> persist RECORDING snapshot
  |     -> return session_id + epoch + RECORDING only after persistence succeeds
  |
  +-- extension capture guard applies helper authority
  |
  +-- attach recording-scoped browser listeners
  |
  +-- persist current-active-view navigation/privacy + visibility snapshot
  |
  `-- recording UI becomes active only if the snapshot path succeeds
```

Do not mark UI as recording until helper authority and the initial observable-view snapshot path are established. A durably represented explicit gap may satisfy the snapshot path; silent loss may not.

## 4. Pause/Stop flow and asynchronous races

Pause/Stop is helper-authoritative, but browser acquisition is shut off locally at the user-control boundary.

1. user requests Pause/Stop;
2. extension immediately suspends local capture authority and detaches raw browser observation listeners **before** awaiting any helper control round trip;
3. already-queued async work now holds an invalid capture token and is discarded before browser-result use/event materialization;
4. helper validates transition and increments epoch;
5. helper commits the non-recording authority snapshot; if persistence fails while the previous state was `RECORDING`, helper revokes in-memory authority to `INTERRUPTED` and returns the current safe state;
6. extension applies returned non-recording state/new epoch;
7. browser listeners remain detached until an explicit helper-authorized Resume succeeds;
8. Resume reattaches listeners and creates a fresh view segment/current-view snapshot so the paused interval cannot be interpreted as continuous exposure.

A control-message failure must not make the extension continue optimistically. If authority becomes uncertain, fail closed and surface Interrupted/helper-error state.

## 5. Helper disconnect / restart flow

```text
Native port disconnects
  -> detach raw browser observation listeners immediately
  -> extension capture guard invalidates connection generation immediately
  -> stop initiating observations
  -> discard in-flight raw/high-fidelity material
  -> do not queue page text/screenshots for reconnect
  -> show interrupted/helper unavailable state
  -> reconnect performs fresh handshake
  -> helper opens durable authority journal
  -> persisted RECORDING/PAUSED from an unfinished helper lifetime
       becomes durable INTERRUPTED + fresh epoch before handshake exposure
  -> explicit Resume/Stop is required according to product lifecycle
```

A browser process failure while the same helper process and authority remain alive is distinct from helper/OS restart. The lifecycle contract, not browser process residency alone, decides whether the same session may continue.

## 6. Persistence flow

Only a helper-accepted, schema/privacy-valid observation reaches durable browsing-observation persistence.

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

Helper-authoritative control metadata follows a separate bounded state journal and never contains browsing observation payloads.

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

The TypeScript production-extension job covers permission/start gating, helper-authoritative lifecycle controls, Native Messaging disconnect/timeout behavior, capture-token races, observation acknowledgement failure semantics, recording-only browser listener attachment, navigation/visibility/privacy event production, URL secret sanitization, private/internal non-leakage, Start/Resume view snapshots, and Pause/Resume/disconnect listener lifecycle.

The Go production-helper job includes durable observation-store, encrypted-key lifecycle, durable-authority, and platform-path tests for reopen, idempotency, partial-tail recovery, corruption rejection, codec mismatch, authenticated tamper rejection, key rotation, key-state corruption, restart `RECORDING`/`PAUSED` -> `INTERRUPTED`, authority persistence failures, platform base selection, managed-directory containment/symlink rejection, session deletion, and POSIX access modes where applicable.

As production modules are added, their tests join the appropriate production job rather than replacing the independent reference oracles.

### Browser integration after #8 environment is available

- runtime permission grant/refusal;
- normal HTML capture/redaction;
- iframe cases;
- PDF/restricted surfaces;
- screenshot timing/rate;
- helper Native Messaging end-to-end;
- verify raw browser listeners are absent before Start and throughout Pause/Idle;
- Pause/Stop/disconnect while screenshot/capture is in flight;
- helper restart while a session was durably `RECORDING` or `PAUSED`, confirming handshake exposes only `INTERRUPTED` with a fresh epoch.

### Hard privacy tests

Synthetic canaries must never appear in forbidden durable/log/external outputs. The authority journal must be inspected independently to confirm that it contains control metadata only.

See `docs/EVALUATION.md`.

## 8. M1 implementation order and current status

1. **Protocol/schema/reference invariants** — completed baseline.
2. **Extension capture-authority guard** — completed reference and production-state-machine baseline.
3. **Native protocol client abstraction** — completed browser-independent production baseline.
4. **Observation validation + store interface** — completed production baseline.
5. **M1 durable event storage** — per-session framed `FileStore`, AES-256-GCM record codec, and system-key lifecycle core implemented/tested; native OS secret-store adapter remains active platform work.
6. **Durable helper session authority / restart recovery** — production baseline implemented/tested.
7. **Platform-local storage path policy + helper composition** — resolver/layout validation and authoritative production bootstrap are implemented/tested; OS key adapter and safe entrypoint activation remain pending.
8. **Minimal browser control plane + permission flow** — production popup/onboarding/lifecycle baseline implemented/tested; keep this surface intentionally small.
9. **CLI inspector core** — separate `curiotrace` binary with status + single-session human/JSON inspection core implemented/tested; encrypted production reader/bootstrap remains pending.
10. **Browser event collector** — navigation/visibility/privacy/gap baseline implemented/tested in CI; real-browser verification, interaction capture, persistent exclusions, and OS lock/suspend remain pending.
11. **Capture adapter** — integrate #8 findings; normal HTML first.
12. **End-to-end Chrome/Edge session** — Start -> record -> Stop -> inspect.
13. **M1 privacy/restart/adversarial verification**.

## 9. M1 stop conditions

Do not claim M1 complete if any of the following remains true:

- host permission refusal can still create a recording session;
- raw URL-bearing browser observation listeners remain attached before Start or while IDLE/PAUSED/FINISHED/INTERRUPTED;
- helper disconnect leaves capture running or browser observation listeners attached;
- old-epoch async results can be persisted;
- a rejected durable observation can be silently dropped while browser recording continues;
- unredacted raster crosses the extension -> helper boundary on the normal semantic path;
- blocked/private/editable canaries reach durable data;
- unvalidated protocol data can reach the durable store through an ordinary production API;
- normal recording can start with a plaintext/testing durable codec or a generic secret-store fallback;
- production helper can enter `RECORDING` without durable authority persistence;
- unfinished session state can silently recover as `RECORDING` after helper/OS restart;
- authority journal corruption is silently repaired beyond an incomplete trailing write;
- durable browsing data can fall back to cwd/temp/roaming storage because the platform-local root is unavailable;
- browser restart/recovery semantics contradict the product lifecycle;
- #8 empirical tests contradict the selected capture adapter behavior;
- a session cannot be inspected without an LLM/network connection.

## 10. Deliberately deferred choices

M1 planning does not yet fix:

- frontend framework (if any);
- exact third-party/native OS secret-store adapter implementation;
- OCR engine mix;
- installer/update framework;
- any optional richer GUI beyond the minimal browser control plane;
- final capture debounce constants.

The current TypeScript extension + Go helper baseline is documented in `docs/IMPLEMENTATION_STACK.md`; changing that engineering baseline does not alter the product contract. The current per-session durable-log and authority-journal formats are likewise implementation baselines and may be migrated later without changing the product's observation/privacy/lifecycle semantics.

Deferred choices should be made from evidence/maintenance needs, not accidentally encoded into product semantics.
