# CurioTrace Native Helper Protocol

Status: product-level protocol contract for #23. Concrete production language/runtime remains open.

## 1. Purpose

This document defines the trusted local boundary between the CurioTrace browser extension and native helper.

The protocol exists to make privacy/session guarantees enforceable by construction:

- the helper is the authority for global session identity/state;
- the extension captures only while it holds current recording authority;
- stale/late observations are rejectable;
- unredacted screenshots and editable-field values do not cross the process boundary;
- helper loss fails closed instead of silently degrading into partial recording;
- Chrome/Edge/Firefox share one semantic contract even if host installation details differ.

Native Messaging is the baseline transport. Chrome currently frames messages as UTF-8 JSON prefixed by a native-endian 32-bit length, permits up to 64 MiB extension -> host, and limits host -> extension to 1 MiB. Firefox uses the same general stdio/JSON framing and also limits host -> addon responses to 1 MB. Treat these as transport ceilings, not normal payload targets.

## 2. Trust boundary

### Browser extension

Trusted to:

- determine browser/tab/view context;
- enforce `IDLE`/`RECORDING`/`PAUSED` capture gating locally;
- apply browser-visible privacy rules before observation;
- extract allowed visible DOM semantics;
- identify editable/sensitive mask geometry where possible;
- transiently capture a tab viewport;
- redact known sensitive regions before any raster crosses to the helper;
- reduce DOM-inaccessible visual surfaces to low-information fingerprints when required by policy.

The extension is **not** trusted to keep authoritative cross-browser session state by itself.

### Native helper

Trusted CurioTrace code, responsible for:

- authoritative session ID/state/recording epoch;
- accepting/rejecting observations;
- durable private storage and lifecycle enforcement;
- optional OCR/image processing of already-redacted raster data;
- content matching/exposure analysis;
- finalization/summarization state;
- MCP integration for external frontier-model summarization.

The helper does not broaden CurioTrace into an OS-wide screen recorder.

## 3. Fail-closed session authority

A capture is valid only when **both** conditions hold:

1. extension-local state says recording is active; and
2. the extension has a live helper connection plus the helper's current `session_id` and `recording_epoch`.

If the native port disconnects, protocol state becomes ambiguous, version negotiation fails, or the helper reports a non-recording state:

- stop content acquisition immediately;
- stop DOM observers/content-script capture work;
- discard unsent raw/high-fidelity payloads;
- do not buffer page text/screenshots waiting for reconnection;
- surface a recoverable helper/interrupted state to the user;
- require explicit recovery consistent with the session lifecycle contract.

## 4. Recording epoch

Every recording-authorized generation has a monotonically changing `recording_epoch` (an integer or opaque monotonic generation token).

The epoch changes whenever capture authority changes materially, including at least:

- Start;
- Pause;
- Resume;
- Stop;
- transition to Interrupted;
- helper recovery that cannot prove continuity safely.

Every observation message carries the `session_id` and `recording_epoch` under which it was produced.

The helper accepts content-bearing observations only when both exactly match the current authoritative recording session/epoch.

Late messages from an old epoch are rejected and their transient payload is discarded. They must not mutate durable session state.

This is the primary defense against Pause/Stop races and delayed asynchronous screenshot/OCR work.

## 5. Protocol envelope

Every extension <-> helper message uses a versioned logical envelope:

```json
{
  "protocol_version": "1.0",
  "message_id": "01J...",
  "kind": "observation.semantic",
  "session_id": "ses_...",
  "recording_epoch": 7,
  "browser_instance_id": "br_...",
  "payload": {}
}
```

Fields may be omitted only when not meaningful for the message kind (for example pre-session handshake).

### Required semantics

- `protocol_version`: major/minor protocol version.
- `message_id`: unique ID for diagnostics/idempotency within a connection.
- `kind`: closed, versioned message-kind namespace.
- `session_id`: authoritative CurioTrace session identity where applicable.
- `recording_epoch`: current capture-authority generation where applicable.
- `browser_instance_id`: helper-scoped browser/extension instance identity, not a global tracking identifier.
- `payload`: kind-specific body.

Unknown incompatible major versions fail closed.

Minor-version capability negotiation may permit backward-compatible optional fields, but privacy semantics may not silently weaken because one side is older.

## 6. Connection handshake

A long-lived `connectNative()` style connection is preferred while CurioTrace is active.

Conceptual handshake:

### Extension -> helper

```json
{
  "protocol_version": "1.0",
  "message_id": "...",
  "kind": "hello",
  "payload": {
    "extension_version": "...",
    "browser_family": "chromium",
    "browser_version": "...",
    "platform": "...",
    "capabilities": [
      "semantic_dom_v1",
      "redacted_raster_v1",
      "visual_fingerprint_v1"
    ]
  }
}
```

### Helper -> extension

```json
{
  "protocol_version": "1.0",
  "message_id": "...",
  "kind": "hello.ack",
  "payload": {
    "helper_version": "...",
    "compatible": true,
    "required_capabilities": [],
    "session_state": "IDLE",
    "session_id": null,
    "recording_epoch": 12
  }
}
```

If either side cannot satisfy a required privacy/capture capability, recording must not start.

## 7. Allowed extension -> helper data classes

### 7.1 `control`

Lifecycle, health, capability and state transition requests.

Examples:

- start request;
- pause/resume/stop request;
- heartbeat/health;
- capability negotiation;
- explicit user exclusion-rule update.

### 7.2 `metadata`

Policy-allowed browser/session metadata:

- URL/canonical URL where allowed;
- title where allowed;
- navigation/view identity;
- tab/window activation state;
- visibility/focus facts;
- wall-clock and monotonic/session-relative timing;
- capture method/status;
- gap/error markers.

Excluded/private pages follow the product privacy contract and may not include URL/title/content simply because the transport supports them.

### 7.3 `semantic_observation`

Privacy-filtered content evidence from inspectable surfaces:

- visible non-editable text/semantic units;
- allowed structural/geometry information;
- allowed selection/copy observations;
- uncertainty/capture gaps;
- provenance needed for later content identity/exposure analysis.

This must not contain editable form values.

### 7.4 `redacted_raster`

A transient raster derived from `captureVisibleTab()` **only after known sensitive/editable regions have been redacted inside the extension process**.

Requirements:

- carry a redaction-policy/version identifier;
- carry viewport dimensions/scale metadata required to interpret it;
- be session+epoch scoped;
- remain transient in the helper unless a separately authorized feature explicitly says otherwise;
- never be duplicated into logs/crash reports;
- be discarded after OCR/fingerprint processing.

An unredacted screenshot is not a valid `redacted_raster` payload.

### 7.5 `visual_fingerprint`

Low-information/non-reversible visual identity data for Tier-2 surfaces where semantic DOM inspection is unavailable and raw pixels must not cross the boundary.

May include:

- perceptual hash/fingerprint;
- viewport dimensions;
- change-distance metrics;
- timing/source metadata allowed by policy.

Must not contain:

- image bytes;
- OCR text;
- reconstructable thumbnails;
- editable-field contents.

## 8. Forbidden default extension -> helper payloads

The baseline protocol does not permit:

- unredacted screenshot/raster bytes;
- password values;
- authentication/session tokens;
- payment/security-code values;
- ordinary editable form/input/textarea/contenteditable values;
- global/system clipboard contents;
- excluded/private-page content;
- excluded-page URL/title where the privacy contract forbids persistence;
- arbitrary filesystem paths/capabilities granting unrelated file access;
- external-provider credentials unless a later explicitly designed integration requires them.

A future feature that needs a currently forbidden class requires a new product/privacy decision and protocol capability, not an undocumented field.

## 9. Tier-2 DOM-inaccessible surfaces

For an otherwise allowed viewport where semantic inspection cannot establish editable/sensitive geometry (for example a built-in PDF viewer):

1. capture pixels transiently in extension memory only if needed for visual identity;
2. compute the visual fingerprint in the extension where feasible;
3. send only `visual_fingerprint` + allowed metadata to the helper;
4. do not send the raw raster merely to let the helper hash it;
5. do not OCR the unredacted raster by default;
6. discard the raster immediately after fingerprint extraction.

If safe in-extension fingerprinting is not technically viable on a target browser, fail closed for visual content until a separately reviewed safe path exists.

## 10. Helper -> extension messages

Keep return traffic small and non-content-heavy, both for privacy minimization and Native Messaging response-size limits.

Allowed baseline responses:

- current session state;
- current session ID/epoch;
- capability/compatibility result;
- observation accepted/rejected acknowledgement;
- rejection reason code;
- capture backpressure/rate guidance;
- safe diagnostics suitable for user-visible repair UI;
- summarization/finalization state summary where useful.

Do not send processed screenshots back to the browser in the baseline design.

## 11. Observation acknowledgement

Content-bearing messages should receive an acknowledgement containing at least:

```json
{
  "kind": "observation.ack",
  "payload": {
    "accepted": false,
    "reason": "STALE_EPOCH"
  }
}
```

Representative reason codes:

- `OK`
- `NOT_RECORDING`
- `STALE_EPOCH`
- `UNKNOWN_SESSION`
- `INCOMPATIBLE_PROTOCOL`
- `INVALID_SCHEMA`
- `PRIVACY_VIOLATION`
- `PAYLOAD_TOO_LARGE`
- `BACKPRESSURE`

Rejecting an observation must not cause the helper to persist rejected content payloads for debugging.

## 12. Large raster transport

Chrome's documented extension -> native-host message ceiling is large enough for ordinary compressed viewport images, but CurioTrace should keep normal payloads far below the ceiling.

Baseline:

- encode already-redacted viewport images in a bounded compressed format;
- send in-memory over Native Messaging;
- do not create shared temp screenshot files solely for IPC;
- reject unexpectedly large frames before transport;
- apply backpressure and event coalescing rather than queueing many images.

If future measurements require chunking:

- chunk protocol must be explicitly versioned;
- every chunk is session+epoch+transfer-ID scoped;
- total transfer size is declared and bounded;
- integrity is checked before processing;
- partial transfers are discarded on timeout/disconnect/epoch change;
- no chunk is independently persisted as a durable screenshot artifact.

## 13. Logging and diagnostics

Neither side may log captured page text, raster bytes, selected text, URL query contents containing sensitive data, or provider-bound source content by default.

Diagnostics should use:

- message kind;
- byte length;
- reason/error code;
- timing;
- session/event IDs where safe;
- redaction/fingerprint method versions;
- browser/helper versions.

Debug builds that expose payloads require explicit developer action and must not become the production default.

## 14. Reconnect/recovery

A new native connection performs a fresh handshake.

The extension must not assume that a previous `RECORDING` authority remains valid merely because it reconnected.

The helper decides whether safe continuity can be proven. Otherwise the session follows the product `INTERRUPTED` recovery path and requires explicit user action.

## 15. Protocol testing requirements

Browser-independent tests must cover at minimum:

- valid handshake;
- incompatible protocol major version;
- helper disconnect while recording;
- stale epoch observation after Pause/Stop;
- privacy-invalid semantic payload;
- `fingerprint_only` containing forbidden image/text data;
- `redacted_raster` missing redaction-policy metadata;
- excluded/private observation carrying forbidden URL/title/content;
- oversized payload rejection;
- late chunk/observation after epoch change;
- helper acknowledgements never echoing sensitive payloads.

See #24/#25.

## 16. References

- Chrome Native Messaging: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging
- MDN Native Messaging: https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Native_messaging
- Capture architecture: `docs/CAPTURE_ARCHITECTURE.md`
- Product contract: `docs/PRODUCT_SPEC.md`

Concrete serialization/library choices remain implementation details as long as they preserve this contract.