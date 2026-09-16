# CurioTrace Capture Architecture Spike

Status: **leading recommendation; empirical browser matrix still required before #8 closes**.

This document records the architecture narrowed down by issue #8 after the product/privacy/copyright boundaries in #1–#7 and #18–#21 were fixed.

## 1. Decision criteria

A capture architecture must satisfy all of the following simultaneously:

- reconstruct what was actually visible rather than simply archive whole documents;
- keep privacy filtering as early as technically feasible;
- never depend on semantic secret detection as a hard guarantee;
- avoid durable raw screenshots/full-page mirrors by default;
- retain enough compact historical evidence for `session.md`;
- support Chrome/Edge first and Firefox parity later;
- remain feasible on Windows/macOS/Linux through the native-helper boundary;
- avoid high-rate screenshot/video capture;
- keep external frontier-model summarization separate from recording;
- preserve a clean path to downstream LLM Wiki ingestion.

## 2. Candidate comparison

| Candidate | Visible-content fidelity | Privacy masking | Structure / interactions | PDF/canvas/image text | CPU / storage | Portability | Result |
|---|---|---|---|---|---|---|---|
| DOM-only WebExtension | Good on ordinary HTML, imperfect for rendered pixels | Strong where DOM is inspectable | Strong | Weak | Low | Strong | Reject as sole capture path |
| Screenshot + OCR for everything | Strong pixel fidelity | Weakest; requires raster redaction before OCR | Weak; must reconstruct structure | Strong | High | Moderate | Reject as default path |
| OS-wide screen recorder | Very broad | Unacceptably broad capture scope | Weak browser semantics | Strong | High | Poorer / invasive | Reject |
| Browser screenshot + local OCR only | Good viewport fidelity | Better than OS-wide but still raster-first | Weak | Strong | Medium-high | Strong-ish | Reject as primary path |
| **Hybrid WebExtension + native helper** | **Strong where needed** | **DOM-aware on normal pages; fail-closed tiers elsewhere** | **Strong** | **Fallback coverage** | **Adaptive** | **Strong semantic model** | **Leading recommendation** |

## 3. Leading architecture

### Browser extension

Owns browser-context information that the native helper cannot safely infer:

- Start/Pause/Stop UX;
- tab/window/navigation/visibility events;
- runtime host-permission onboarding;
- dynamically registered recording-time content scripts;
- visible DOM text/geometry where permitted;
- editable/sensitive region geometry;
- selection/copy observations allowed by the privacy contract;
- active-tab viewport screenshot acquisition after the privacy gate;
- event-driven scheduling and capture eligibility decisions.

### Native helper

Acts as CurioTrace's trusted local backend, not as an OS-wide screen recorder:

- session state authority shared across supported browsers;
- application-private temporary and durable storage;
- immediate raster redaction / image processing where used;
- fallback OCR;
- visual fingerprints/content matching/exposure analysis where appropriate;
- encryption/cleanup/lifecycle enforcement;
- MCP server for external frontier-model summarization;
- generation/finalization state for `session.md`.

Native Messaging is preferred over an unauthenticated localhost HTTP service for extension-to-helper communication.

## 4. Permission model

### Install time

Do not require broad HTTP/HTTPS host access merely because the extension is installed.

### First explicit Start

Use MV3 runtime optional host permissions to explain and request the web-page access required for zero-friction cross-site session recording.

Chrome and Firefox both support `optional_host_permissions` / `permissions.request()`.

The browser-granted permission is capability, not recording authorization. CurioTrace's own session state remains stricter:

- `IDLE`: no capture;
- `RECORDING`: capture allowed surfaces;
- `PAUSED`: no capture.

A missing required host permission must not silently produce a session represented as complete.

## 5. Recording-scoped content scripts

Use `scripting.registerContentScripts()` at Start and unregister on Pause/Stop where practical. Set `persistAcrossSessions: false`.

Chrome and Firefox support dynamic registration in MV3.

Important: unregistering does not remove already-injected code from an open page. Therefore injected scripts must contain their own explicit inactive state and stop observers when recording authorization is removed. Privacy must never depend solely on unregistering future injections.

## 6. Three capture tiers

### Tier 1 — semantic viewport capture

Use on ordinary pages where enough DOM/geometry is inspectable.

Capture/derive:

- visible non-editable DOM text;
- page/source metadata;
- selection/copy observations;
- viewport/element geometry;
- editable/sensitive rectangles;
- optional transient viewport raster for visual gaps/fingerprinting.

For a transient raster:

1. capture active-tab pixels;
2. redact known sensitive/editable regions inside CurioTrace's trusted local pipeline;
3. OCR/hash only the redacted representation;
4. persist only allowed derived/compact information;
5. discard raster data.

OCR should be a fallback for rendered information DOM cannot represent well, not the normal path for ordinary HTML.

### Tier 2 — fingerprint-only visual capture

Use on an allowed surface whose viewport can be captured but whose DOM cannot be safely inspected, particularly a browser built-in PDF viewer.

Because CurioTrace cannot reliably find PDF form/editable regions there, default behavior is:

- retain allowed URL/title/navigation/exposure metadata;
- transiently capture the viewport only for visual-change/content-identity fingerprinting;
- do **not** OCR the unredacted raster;
- do not persist pixels;
- discard pixels immediately after fingerprint extraction;
- record that semantic viewport content was unavailable because of the privacy boundary.

This preserves some exposure/revisit information without retaining PDF text or possible typed form values.

The downstream LLM Wiki may independently follow the source URL later. That is a new source acquisition and must not be represented as content the user necessarily saw.

### Tier 3 — no-content capture

Use for configured exclusions, private/incognito contexts, browser-internal pages, and other surfaces disallowed by the privacy contract.

No DOM text, raster, OCR, or visual fingerprint is retained. Only minimal non-content continuity information allowed by the product spec may remain.

## 7. Cross-origin / sandboxed frames

For a frame where CurioTrace can safely inject and has permission, inspect it independently.

If frame content cannot be inspected reliably:

- do not assume its pixels are safe;
- use the top-frame iframe rectangle as a conservative redaction region when performing semantic screenshot/OCR processing;
- keep uncertainty explicit;
- do not silently attribute inaccessible frame text to the top page.

The empirical matrix must test normal cross-origin, sandboxed, nested, and opaque-origin cases.

## 8. Redaction transport

The current PoC uses visible magenta overlays only to prove geometry/masking feasibility. That is not preferred product UX.

Preferred production direction is transient screenshot -> trusted local redaction.

Native Messaging message-size limits make extension-to-helper viewport transfer technically plausible:

- Chrome: up to 64 MiB per message sent from extension to native host;
- Firefox: documented extension-to-host limit is much larger;
- native host -> browser is limited to 1 MiB, so processed images should remain in the helper and only small metadata/results should return.

This must still be benchmarked for latency/memory. Raw pixels must never be logged or durably written as part of the default path.

## 9. Capture scheduling

Do not poll screenshots like video.

Use event-driven signals such as:

- navigation;
- active-tab/window visibility changes;
- scroll;
- resize;
- DOM mutation;
- selection/copy;
- periodic low-frequency verification only where necessary for canvas/visual-only surfaces.

Debounce/coalesce events and stay comfortably below browser screenshot-rate limits. Exposure intervals should usually be derived from state continuity between meaningful change events, not from counting screenshots.

## 10. Copyright consequence

CurioTrace captures the viewport/session evidence, not an entire document archive.

- Do not fetch a complete page/PDF merely because the user viewed part of it unless a later explicitly justified feature requires it.
- Full OCR/page text is ephemeral processing material, not durable storage.
- Persist URL/title/time, exposure/revisit evidence, compact observed semantics, and only limited justified exact excerpts.
- Let the downstream LLM Wiki independently retrieve source URLs when appropriate.

## 11. Empirical matrix still required

Issue #8 must not close until a normal developer browser environment runs the PoC and records at least:

| Browser/surface | host permission | DOM works | sensitive geometry known | screenshot works | mask/redaction feasible | capture latency | notes |
|---|---|---:|---:|---:|---:|---:|---|
| Chrome ordinary HTML | pending | pending | pending | pending | pending | pending | |
| Edge ordinary HTML | pending | pending | pending | pending | pending | pending | |
| Chrome PDF viewer | n/a/restricted | expected no | expected no | pending | fingerprint-only candidate | pending | |
| Edge PDF viewer | n/a/restricted | expected no | expected no | pending | fingerprint-only candidate | pending | |
| same-origin iframe | pending | pending | pending | n/a | pending | n/a | |
| cross-origin iframe | pending | pending | pending | n/a | pending | n/a | |
| sandboxed/nested iframe | pending | pending | pending | n/a | pending | n/a | |
| canvas/image text | pending | DOM gap expected | surrounding geometry only | pending | pending | pending | |
| SPA/infinite scroll | pending | pending | pending | pending | pending | pending | |
| restricted browser page | n/a | expected no | no | browser-dependent | should be denied | n/a | |
| Firefox parity check | pending | pending | pending | pending | pending | pending | |

The repository PoC lives under `spikes/capture-poc/`.

## 12. What would invalidate the recommendation

Reconsider the hybrid recommendation if empirical testing shows any of the following:

- runtime permissions cannot support the intended recording UX across target browsers;
- screenshot capture routinely includes pixels outside the intended active-tab content scope;
- DOM geometry cannot support reliable redaction on ordinary pages;
- native-message image transfer is too slow/memory-heavy for reasonable event-driven use;
- cross-origin frame behavior forces unacceptable data loss or unsafe capture;
- Firefox parity requires a substantially different semantic architecture rather than implementation adaptation.

Until those tests complete, this is a strong architecture hypothesis, not a final implementation commitment.
