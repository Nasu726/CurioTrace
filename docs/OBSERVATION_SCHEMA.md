# CurioTrace Observation Schema v1

Status: M1 semantic event contract for #24. Transport-independent and browser-independent.

## 1. Purpose

The observation schema defines the durable/replayable meaning of a CurioTrace session.

It is intentionally separate from:

- WebExtension APIs;
- Native Messaging framing;
- database/file format;
- OCR implementation;
- Markdown generation.

A Chrome, Edge, or Firefox implementation should produce events with the same semantics.

## 2. Common envelope

Every durable event uses this conceptual envelope:

```json
{
  "schema_version": "1.0",
  "event_id": "evt_01J...",
  "session_id": "ses_01J...",
  "recording_epoch": 7,
  "event_type": "content_observation",
  "wall_time": "2026-09-16T09:30:00.123Z",
  "monotonic_ms": 18342.51,
  "browser_instance_id": "br_...",
  "view_id": "view_...",
  "capture_mode": "dom",
  "source": {
    "url": "https://example.com/article",
    "title": "Example article"
  },
  "payload": {}
}
```

### Required common fields

- `schema_version`: schema major/minor version.
- `event_id`: unique durable event identity.
- `session_id`: owning CurioTrace session.
- `recording_epoch`: capture-authority generation from the native-helper contract.
- `event_type`: typed event class.
- `wall_time`: human chronology timestamp.
- `monotonic_ms`: duration-safe session-relative/monotonic timestamp.

### Context fields

- `browser_instance_id`: identifies the browser instance for session-local continuity; not a global user tracking identifier.
- `view_id`: CurioTrace view/navigation identity. Runtime browser tab IDs may be stored only as implementation/debug metadata and are not durable global identity.
- `capture_mode`: capture semantics for observation/capture events.
- `source`: URL/title/source pointer only when permitted by privacy policy.
- `payload`: event-specific data.

## 3. Event classes

### 3.1 `session_state`

Records authoritative lifecycle transitions.

Payload example:

```json
{
  "from": "IDLE",
  "to": "RECORDING",
  "reason": "USER_START"
}
```

Allowed states include:

- `IDLE`
- `RECORDING`
- `PAUSED`
- `FINISHED`
- `INTERRUPTED`

Post-Stop summarization states are separate derived/finalization state and need not masquerade as browsing exposure events.

### 3.2 `navigation`

Records allowed browser/view transitions.

Payload may include:

- transition kind;
- previous/new `view_id`;
- same-document/SPА marker where known;
- redirect/reload marker;
- allowed source pointer.

An excluded/private destination follows the privacy contract: do not persist its URL/title merely because navigation occurred.

### 3.3 `visibility`

Records evidence needed for exposure reconstruction.

Payload examples:

- active tab became visible;
- tab moved to background;
- browser window minimized/restored when known;
- OS lock/suspend gate activated;
- visibility became unknown.

Unknown visibility is an explicit state, not equivalent to visible or invisible.

### 3.4 `content_observation`

Records compact evidence of what was actually observable in an allowed view.

Allowed capture modes:

- `dom`
- `redacted_visual`
- `fingerprint_only`
- `metadata_only`
- `blocked`
- `failed`

#### `dom`

May contain compact privacy-filtered visible semantic units and geometry/provenance.

Must not contain editable form values.

#### `redacted_visual`

Durable event does **not** contain raster bytes. It records results/provenance derived from a transient already-redacted raster, such as OCR fallback units, image-processing method/version, and gaps.

The raster itself is transient transport/processing material governed by `docs/NATIVE_HELPER_PROTOCOL.md`.

#### `fingerprint_only`

May contain low-information fingerprint identifiers/distances and viewport metadata.

Must not contain:

- image bytes;
- thumbnail data;
- OCR text;
- semantic text claimed to come from the inaccessible pixels.

#### `metadata_only`

No content semantics. Used when only allowed URL/title/timing/view facts are available.

#### `blocked`

No content and, where the privacy contract requires it, no URL/title/source pointer.

May retain only allowed continuity data such as timestamp, exclusion reason, and non-content rule identifier.

#### `failed`

Records an attempted allowed capture that failed. It must represent the gap/error explicitly rather than synthesizing content.

### 3.5 `interaction`

Allowed explicit user interaction evidence, initially selection/copy observations.

Payload should distinguish:

- event type (`selection`, `copy`);
- best-supported target observation/view reference;
- exact selected non-editable page text when policy allows;
- unresolved/ambiguous target state.

CurioTrace does not read the global clipboard and does not persist copied form values.

### 3.6 `privacy_decision`

Records the outcome of privacy/capture gating without leaking the denied content.

Examples:

- domain exclusion matched;
- private/incognito denied;
- editable region masked;
- inaccessible frame conservatively redacted;
- browser-internal surface denied.

Do not encode the forbidden payload into diagnostic text.

### 3.7 `capture_status`

Records capture method/version and status useful for interpreting later gaps.

Examples:

- DOM probe available/unavailable;
- screenshot capture succeeded/failed;
- redaction version;
- visual fingerprint algorithm version;
- fallback path chosen.

### 3.8 `gap_error`

Records known missing or uncertain data.

Examples:

- helper disconnected;
- unsupported browser surface;
- permission missing;
- frame inaccessible;
- capture throttled/backpressured;
- observation discarded as stale epoch.

A gap is not evidence that nothing changed.

## 4. Source pointer

When policy permits, a source pointer may contain:

```json
{
  "url": "https://example.com/page",
  "title": "Example",
  "canonical_url": "https://example.com/page"
}
```

Rules:

- source pointers identify the work/view, not proof that its entire contents were observed;
- downstream re-fetch is a separate acquisition and must not be merged into historical observation claims;
- excluded/private surfaces may forbid even URL/title persistence;
- implementations should avoid unnecessary URL fragments/query secrets in durable storage where a safer normalized source pointer can preserve meaning, but normalization must not silently destroy provenance needed by the product.

## 5. Compact semantic units

A `dom` or allowed `redacted_visual` observation may contain compact semantic units such as:

```json
{
  "units": [
    {
      "unit_id": "u_1",
      "kind": "text",
      "text": "A short visible semantic unit.",
      "rect": {"x": 12, "y": 80, "w": 640, "h": 40},
      "confidence": 1.0
    }
  ]
}
```

M1 does not require a perfect canonical unit format. The important invariants are:

- units correspond to actually observed allowed content;
- editable/sensitive values are absent;
- provenance to the event/view remains possible;
- uncertainty is retained;
- near-complete third-party page copies are not treated as default durable storage.

## 6. Privacy/schema invariants

Validation is layered. JSON Schema checks shape; a semantic validator checks cross-field policy.

Mandatory semantic invariants:

1. Content-bearing events are accepted only for the helper's current `session_id` + `recording_epoch` while state is `RECORDING`.
2. `blocked` observations cannot carry semantic/content payloads.
3. Excluded/private events cannot carry URL/title/content where the product privacy contract forbids it.
4. `fingerprint_only` cannot carry image bytes, thumbnails, OCR text, or semantic units derived from inaccessible pixels.
5. Durable `redacted_visual` events cannot contain raster bytes; raster transport is transient only.
6. `dom`/semantic events cannot contain editable form/input/textarea/contenteditable values.
7. Password/token/payment/security-code values are never valid observation payloads.
8. Unknown incompatible schema major versions fail closed.
9. `wall_time` and `monotonic_ms` are both required for durable events.
10. Missing/unsupported capture is represented explicitly as `failed`, `metadata_only`, `blocked`, or `gap_error` rather than fabricated semantic content.
11. Later derived analysis references source `event_id`s instead of rewriting source events into a new interpretation.
12. Rejected privacy-invalid/stale observations must not be persisted merely for debugging.

## 7. Versioning

Schema version uses `major.minor` semantics.

- major change: interpretation/privacy meaning can be incompatible;
- minor change: backward-compatible optional fields/event detail.

Readers must reject an unknown major version unless an explicit migration/adaptation layer understands it.

Storage migrations may change representation but must preserve documented event meaning/provenance or explicitly record irrecoverable loss.

## 8. Payload size

Observation payloads are bounded.

Large page text/images are not justified simply because transport/storage permits them. Oversized content should be compacted into bounded semantic units or transient processing artifacts according to the product/copyright policy.

Exact implementation limits are benchmark/configuration choices, but M1 validators should have explicit safe upper bounds so malformed pages cannot create unbounded memory/storage growth.

## 9. Example: normal DOM observation

```json
{
  "schema_version": "1.0",
  "event_id": "evt_001",
  "session_id": "ses_001",
  "recording_epoch": 3,
  "event_type": "content_observation",
  "wall_time": "2026-09-16T09:30:00.123Z",
  "monotonic_ms": 12500.2,
  "browser_instance_id": "br_chrome_1",
  "view_id": "view_7",
  "capture_mode": "dom",
  "source": {
    "url": "https://example.com/article",
    "title": "Example article"
  },
  "payload": {
    "units": [
      {
        "unit_id": "u_1",
        "kind": "text",
        "text": "Visible article paragraph summary unit.",
        "confidence": 1.0
      }
    ],
    "capture_method_version": "dom-probe/1"
  }
}
```

## 10. Example: Tier-2 fingerprint-only observation

```json
{
  "schema_version": "1.0",
  "event_id": "evt_002",
  "session_id": "ses_001",
  "recording_epoch": 3,
  "event_type": "content_observation",
  "wall_time": "2026-09-16T09:31:00.000Z",
  "monotonic_ms": 72500.0,
  "browser_instance_id": "br_chrome_1",
  "view_id": "view_pdf_1",
  "capture_mode": "fingerprint_only",
  "source": {
    "url": "https://example.com/paper.pdf",
    "title": "paper.pdf"
  },
  "payload": {
    "visual_fingerprint": "phash-v1:...",
    "viewport": {"width": 1440, "height": 900},
    "semantic_content_unavailable": true,
    "reason": "DOM_INACCESSIBLE_PRIVACY_BOUNDARY"
  }
}
```

## 11. Example: blocked observation

```json
{
  "schema_version": "1.0",
  "event_id": "evt_003",
  "session_id": "ses_001",
  "recording_epoch": 3,
  "event_type": "privacy_decision",
  "wall_time": "2026-09-16T09:32:00.000Z",
  "monotonic_ms": 132500.0,
  "capture_mode": "blocked",
  "payload": {
    "reason": "SITE_EXCLUSION",
    "rule_id": "rule_12"
  }
}
```

There is intentionally no source URL/title/content in this example.

## 12. Relationship to other documents

- IPC/trust boundary: `docs/NATIVE_HELPER_PROTOCOL.md`
- capture tiers: `docs/CAPTURE_ARCHITECTURE.md`
- privacy/copyright/product semantics: `docs/PRODUCT_SPEC.md`
- quality/golden sessions: `docs/EVALUATION.md`

A machine-readable v1 schema and reference semantic validator should implement this contract rather than inventing new meanings.