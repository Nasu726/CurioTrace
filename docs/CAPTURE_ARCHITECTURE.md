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
- **immediate in-extension raster redaction before native transfer where DOM-safe geometry exists**;
- event-driven scheduling and capture eligibility decisions.

### Native helper

Acts as CurioTrace's trusted local backend, not as an OS-wide screen recorder:

- session state authority shared across supported browsers;
- application-private temporary and durable storage;
- OCR / image processing on already-redacted raster inputs where possible;
- visual fingerprints/content matching/exposure analysis where appropriate;
- encryption/cleanup/lifecycle enforcement;
- MCP server for external frontier-model summarization;
- generation/finalization state for `session.md`.

Native Messaging is preferred over an unauthenticated localhost HTTP service for extension-to-helper communication.

## 4. Permission model

### Install time

Do not require broad HTTP/HTTPS host access merely because the extension is installed.

The extension may be installed and its helper may be present without browsing-content capture authorization.

### First explicit Start

Use MV3 runtime optional host permissions for HTTP/HTTPS website access.

Before the browser permission dialog appears, CurioTrace shows its own concise explanation. The explanation must state:

1. **why the access is broad:** one explicit session can navigate across unrelated sites/tabs, and site-by-site permissions would create silent gaps that make the trace unreliable;
2. **when capture happens:** installation/background residency alone does not capture browsing content; capture is gated by explicit `RECORDING` state;
3. **what remains excluded:** private/incognito and configured exclusions remain outside the normal capture scope;
4. **what the permission does not authorize:** website access is not consent to send browsing content to Claude/Codex/another external model; external summarization is a separate authorization boundary;
5. **what denial means:** CurioTrace will not start a partial recording that appears complete.

Only after the user explicitly continues does CurioTrace call the browser runtime permission API.

If the required broad HTTP/HTTPS host access is denied or unavailable, `Start` fails closed: the session does **not** enter `RECORDING`, and no DOM/screenshot capture occurs.

Once granted, browser host permission is only a capability. CurioTrace's own state remains stricter:

- `IDLE`: no capture;
- `RECORDING`: capture allowed surfaces;
- `PAUSED`: no capture.

The product UI/listing must avoid implying that broad permission means CurioTrace continuously reads every website.

Chrome and Firefox support optional host permissions / runtime permission requests; exact prompts and store disclosure requirements remain browser-specific.

## 5. Recording-scoped content scripts

Use `scripting.registerContentScripts()` at Start and unregister on Pause/Stop where practical. Set `persistAcrossSessions: false`.

Chrome and Firefox support dynamic registration in MV3.

Important: unregistering does not remove already-injected code from an open page. Therefore injected scripts must contain their own explicit inactive state and stop observers when recording authorization is removed. Privacy must never depend solely on unregistering future injections.

Firefox MV3 currently uses background scripts/event pages while Chromium uses an extension service worker. Keep the semantic state machine common even where manifest/background implementation differs.

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
2. redact known sensitive/editable/opaque-frame rectangles **inside the extension runtime while the image remains transient**;
3. pass only the redacted representation to the native helper when OCR/image processing is needed;
4. persist only allowed derived/compact information;
5. discard the transient raster representation after the required local processing.

OCR should be a fallback for rendered information DOM cannot represent well, not the normal path for ordinary HTML.

### Tier 2 — fingerprint-only visual capture

Use on an allowed surface whose viewport can be captured but whose DOM cannot be safely inspected, particularly a browser built-in PDF viewer.

Because CurioTrace cannot reliably find PDF form/editable regions there, default behavior is:

- retain allowed URL/title/navigation/exposure metadata;
- transiently capture the viewport only for visual-change/content-identity fingerprinting;
- do **not** OCR the unredacted raster;
- do not persist pixels;
- discard pixels immediately after fingerprint extraction/necessary capture metadata;
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

## 8. Redaction and native transport

The preferred production direction is:

`captureVisibleTab -> extension-memory redaction -> redacted raster/metadata -> Native Messaging -> helper`

Standard Canvas / `OffscreenCanvas` processing makes in-extension redaction technically plausible. The current PoC uses `OffscreenCanvas` to scale DOM mask rectangles into screenshot pixel coordinates and stores only the redacted inspection image on DOM-inspectable pages.

On DOM-unavailable surfaces the PoC does not persist the captured raster; it records only capture success/dimensions and discards the unredacted pixels, matching the Tier-2 direction.

Why redact before the helper:

- it minimizes the lifetime/scope of known sensitive pixels;
- it avoids sending known editable/form/frame pixels over Native Messaging unredacted;
- it makes the extension -> helper interface easier to audit;
- it aligns with browser-store disclosure requirements that treat browsing data/native messaging as sensitive handling/transmission.

Native Messaging message-size limits still need empirical benchmarking for redacted viewport images. Processed images should remain in the helper when possible; return only small metadata/results to the extension.

Raw/unredacted pixels must never be intentionally logged or durably written by the default path.

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

CurioTrace captures viewport/session evidence, not an entire document archive.

- Do not fetch a complete page/PDF merely because the user viewed part of it unless a later explicitly justified feature requires it.
- Full OCR/page text is ephemeral processing material, not durable storage.
- Persist URL/title/time, exposure/revisit evidence, compact observed semantics, and only limited justified exact excerpts.
- Let the downstream LLM Wiki independently retrieve source URLs when appropriate.

## 11. Store/release privacy consequence

Browser-store policy is tracked in #22 and is a public-release gate.

At minimum:

- Chrome store disclosures/privacy policy must state that CurioTrace handles browsing activity/page content as part of its prominent single purpose, even when processing/storage is local;
- Firefox disclosures/consent must account for browsing data sent to the local native helper because Mozilla policy treats Native Messaging as data transmission outside the add-on/local browser;
- the host-permission explanation, store listing, privacy policy, native-helper flow, and actual implementation must agree;
- external frontier-model transfer remains separately authorized from website host access/local native processing.

## 12. Empirical matrix still required

Issue #8 must not close until a normal developer browser environment runs the PoC and records at least:

| Browser/surface | host permission | DOM works | sensitive geometry known | screenshot works | in-extension redaction feasible | capture latency | notes |
|---|---|---:|---:|---:|---:|---:|---|
| Chrome ordinary HTML | pending | pending | pending | pending | pending | pending | |
| Edge ordinary HTML | pending | pending | pending | pending | pending | pending | |
| Chrome PDF viewer | n/a/restricted | expected no | expected no | pending | n/a/fingerprint-only | pending | |
| Edge PDF viewer | n/a/restricted | expected no | expected no | pending | n/a/fingerprint-only | pending | |
| same-origin iframe | pending | pending | pending | n/a | pending | n/a | |
| cross-origin iframe | pending | pending | pending | n/a | pending | n/a | |
| sandboxed/nested iframe | pending | pending | pending | n/a | pending | n/a | |
| canvas/image text | pending | DOM gap expected | surrounding geometry only | pending | pending | pending | |
| SPA/infinite scroll | pending | pending | pending | pending | pending | pending | |
| restricted browser page | n/a | expected no | no | browser-dependent | should be denied | n/a | |
| Firefox parity check | pending | pending | pending | pending | pending | pending | |

The repository PoC lives under `spikes/capture-poc/`.

Static JavaScript syntax checks currently pass; browser-specific API behavior remains empirical.

## 13. What would invalidate the recommendation

Reconsider the hybrid recommendation if empirical testing shows any of the following:

- runtime permissions cannot support the explained first-Start UX across target browsers;
- screenshot capture routinely includes pixels outside the intended active-tab content scope;
- DOM geometry cannot support reliable redaction on ordinary pages;
- in-extension raster redaction is too slow/memory-heavy for reasonable event-driven use;
- redacted native-message image transfer is too slow/memory-heavy;
- cross-origin frame behavior forces unacceptable data loss or unsafe capture;
- Firefox parity requires a substantially different semantic architecture rather than implementation adaptation.

Until those tests complete, this is a strong architecture hypothesis, not a final implementation commitment.
