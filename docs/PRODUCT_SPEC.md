# CurioTrace Product Specification

Status: baseline product semantics before capture-architecture selection.

This document defines user-visible value, responsibility boundaries, privacy guarantees, lifecycle semantics, and completion criteria. Internal implementation is intentionally not fixed unless required to preserve those semantics.

## 1. Product goal

CurioTrace records web browsing during a session that the user explicitly starts, so the session can later be reconstructed and reused without requiring the user to take notes while browsing.

The system should make it possible to recover, as observations rather than guesses:

- which pages were visited;
- which information was actually visible;
- how long information remained exposed;
- where information was shown again or revisited;
- which allowed page content was selected/copied;
- how the user moved between pages;
- when the session started, paused/resumed, ended, or was interrupted.

The primary final artifact is a Markdown representation of a **single session** that gives a downstream LLM enough grounded context to understand what was observed without major omission or fabrication.

CurioTrace also exports a machine-readable representation so future analyzers/renderers can reprocess retained observations.

## 2. Responsibility boundary

CurioTrace owns the pipeline from explicit session recording through single-session export:

1. capture allowed observable browsing events/content;
2. persist source observations locally;
3. derive session-local content identity/exposure/revisit structure;
4. let a human inspect the reconstructed session;
5. export machine-readable session data;
6. generate deterministic Markdown;
7. optionally use an LLM to improve single-session organization/summarization.

CurioTrace does **not** own:

- searching for related past sessions;
- cross-session knowledge synthesis;
- long-term Wiki/knowledge-graph maintenance;
- persistent interest/profile modeling;
- cross-document entity linking as a long-term knowledge base;
- deciding what the user believes, understands, remembers, agrees with, or should do.

Those tasks belong to downstream systems such as an LLM Wiki, coding/research agents, or other knowledge-management tools.

## 3. Observation and inference

CurioTrace separates three layers.

### Observed

Directly recorded facts/events. These are the durable source record and are logically immutable for analysis.

Examples:

- content was observed at a time;
- a tab/view changed;
- a selection/copy event occurred;
- capture failed;
- tracking was paused.

### Derived

Algorithmic results computed from observations.

Examples:

- two observations probably represent the same content;
- an exposure interval lasted approximately a given duration;
- content was redisplayed/revisited;
- normalized text or matching fingerprints.

Derived records must preserve provenance to source observations and express uncertainty where relevant.

### Generated / semantic

Human/LLM-readable organization and summarization of the single session. Generated text is not promoted into the source observation log.

Semantic grouping of observed subject matter is allowed, but unobservable user state must not be asserted as fact. In particular, CurioTrace must not claim as observed fact that the user understood, agreed, solved, remembered, intended, or cared about something.

## 4. Session lifecycle

Application/process state is separate from tracking authorization. A browser extension or helper process may remain enabled or auto-start, but that alone does not authorize content capture.

### States

`IDLE -> RECORDING <-> PAUSED -> FINISHED`

Exceptional state:

`RECORDING/PAUSED -> INTERRUPTED`

Recovery:

`INTERRUPTED -> explicit Resume -> RECORDING`

`INTERRUPTED -> Stop -> FINISHED`

### Rules

- Tracking begins only through explicit `Start`.
- `Start` is zero-friction; no mandatory title/purpose form blocks recording.
- `Pause` preserves session identity but stops browsing-content acquisition.
- `Stop` finalizes the session.
- Before `Start`, while `IDLE`, or while `PAUSED`, browsing URL/content/DOM/screenshot-equivalent observations are not captured.
- Browser closure/crash does not necessarily end the session if the recorder remains alive.
- OS restart or recorder/helper process loss never silently restores `RECORDING`; the unfinished session becomes `INTERRUPTED`.
- OS lock/suspend is not browsing exposure and stops content capture.
- If the recorder survives sleep/suspend with state intact, wake may continue the same recording state.
- Primary Start/Pause/Stop controls must be reachable from the browsing context without opening a full management UI. Browser-extension controls are the baseline surface; tray/global shortcuts may be additional surfaces.
- Recording/excluded state must be readily inspectable while browsing.

## 5. Browser support

Required targets:

- Chrome;
- Edge;
- Firefox (formal parity target after the initial Chrome/Edge implementation stabilizes).

Browser-specific limitations may exist, but downstream session data should share one semantic model rather than separate browser-specific product models.

## 6. Source observation model

The durable history of a session is a logically append-only sequence of privacy-filtered source observations.

Every durable observation has stable event/session identity. The semantic model must represent at least:

- session-state transitions;
- browser/tab/view navigation and activation;
- visibility state needed for exposure reconstruction;
- content observations and capture method/status;
- allowed selection/copy interactions;
- privacy allow/block/mask decisions without leaking excluded content;
- capture/OCR/parser failures and unsupported/gap states.

Runtime browser/tab identifiers are not assumed globally stable across process restarts. When continuity cannot be established reliably, CurioTrace creates a new identity rather than guessing.

Persist both human chronology and duration-safe timing: wall-clock timestamps plus a monotonic/session-relative clock or equivalent. Exposure duration must not depend only on mutable wall-clock differences.

Privacy-filtered raw extracted text remains distinct from normalized/search text. Normalization is derived and must link back to source observations.

Derived records carry analyzer/schema version and provenance. Later re-analysis produces new derived results without rewriting historical source meaning.

Known missing data is represented explicitly rather than treated as evidence that nothing changed or nothing was seen.

User-requested deletion/privacy redaction may physically remove otherwise immutable source payloads. Storage migrations may rewrite representation only while preserving documented meaning/provenance.

## 7. Content identity and exposure

Content identity is derived, not directly observed.

Analyzers may combine text, viewport geometry, visual fingerprints, DOM/accessibility hints, page identity, and temporal continuity. The specific algorithm is replaceable.

### Same content

Observations may be grouped as the same substantive information despite presentation changes such as scrolling, resize, wrapping, rerendering, OCR noise/segmentation, or reload. Material content changes should produce a new/revised identity. Ambiguous matches should remain ambiguous rather than being forced together.

### Continuous exposure

A maximal interval in which the same content is considered visible without evidence of a meaningful non-visible break. Sampling gaps may be bridged only under a documented tolerance. Known Pause/lock/background/minimize/navigation-away intervals end exposure.

CurioTrace must not claim timing precision finer than its observations support.

### Redisplay

Previously visible content becomes non-visible, then visible again, without necessarily leaving the surrounding page context (for example scrolling away and back). Sampling frequency itself must not inflate redisplay counts.

### Revisit

The user leaves the relevant page/view context and later returns to the same page/content context in a later navigation/view segment.

### Visibility caveat

Hidden/background tabs, minimized known-nonvisible browser surfaces, OS lock/suspend, Pause/Idle, and navigation-away periods are excluded from exposure. Focus loss alone is not automatically proof of invisibility because a window may remain visible side-by-side. Unknown occlusion/visibility must be represented as a limitation when the platform cannot establish it.

### Interaction

Allowed selection/copy interactions attach to the best-supported content/source observation. If the exact target is ambiguous, attach at page/view level or mark unresolved.

### Scores

A convenience salience/ranking score may exist later, but it is never the primary record and must remain decomposable into inspectable inputs such as exposure duration, repeat counts, and explicit interactions. It must not be described as a direct measurement of user interest or understanding.

## 8. Privacy contract

Privacy is enforced as early as technically feasible. `capture everything and delete later` is not an acceptable default architecture.

### Exclusion precedence

1. `IDLE` / `PAUSED` / OS lock or suspend: no browsing-content capture.
2. Private/incognito browsing: excluded by default; future support requires separate explicit opt-in.
3. Browser-internal/restricted surfaces that cannot be safely observed: no visual capture.
4. User exclusion rules: deny capture.
5. Otherwise the view is eligible for the normal capture pipeline.

A higher-priority deny cannot be implicitly overridden.

### User controls

At minimum support:

- immediate Pause;
- temporary current-page/view exclusion;
- current-tab exclusion until closed/removed;
- persistent site/domain exclusion.

Advanced URL-pattern rules may be added later.

### Excluded-page persistence

An excluded page may leave only non-content continuity metadata such as timestamp plus exclusion reason/rule identifier. By default do not persist its URL, title, screenshot, OCR/DOM text, form data, or other content-derived metadata. A URL may be inspected transiently in memory only for rule evaluation.

### Forms, credentials, clipboard

- Password values, authentication tokens, payment/security-code values, and browser-managed credential contents must never be intentionally read/persisted.
- Editable form values are not observation content by default, including ordinary inputs/search boxes/textareas/contenteditable regions.
- Known sensitive/editable regions must be redacted before OCR, durable persistence, derived visual-text extraction, or external transfer where the platform provides enough information.
- CurioTrace must not read the global/system clipboard. It may record a browser copy event and allowed selected non-editable page text when directly observable.

### Screenshot boundary

A platform screenshot API may unavoidably expose an unredacted raster frame transiently in process memory. Such frames:

- may exist transiently only when unavoidable;
- are not written to durable storage/logs/crash reports/telemetry by the default pipeline;
- are locally redacted before OCR/persistence/external transfer where identifiable sensitive regions are known;
- require a separate opt-in if a future mode intentionally retains them.

### Navigation / embedded content

Re-evaluate eligibility on navigation, redirects, reload, and observable SPA route changes. Pending capture for a newly excluded view should be discarded before persistence.

Known excluded embedded frames/regions must be masked/omitted where technically possible. If a configured exclusion cannot be reliably masked, visual capture of the affected view fails closed (skip or use a non-visual fallback) rather than knowingly persisting the excluded region.

### Explicit limitation

CurioTrace cannot reliably determine whether arbitrary allowed visible content is confidential. Ordinary text, images, PDFs, canvas/code/chat content, and other unmarked pixels may contain secrets indistinguishable from ordinary content.

The product promise is therefore about capture scope, explicit exclusions, browser-identifiable sensitive inputs, local redaction, retention, and transfer boundaries — not perfect semantic secret detection.

## 9. Data lifecycle

### Durable source observations

Retain locally by default until user deletion or an explicitly configured cleanup policy:

- allowed session/navigation events;
- privacy-filtered raw extracted text;
- source geometry/timing/visibility metadata;
- allowed interaction events;
- capture/error metadata needed to interpret gaps.

### Recomputable derived data

Normalized/search text, content identity, exposure intervals, fingerprints/features, and generated analysis metadata are versioned derived/cache data. They may be discarded/rebuilt from retained source observations.

### Raw visual data

Raw/unredacted screenshots/raster frames are ephemeral by default. Process locally and discard after the required capture/OCR/redaction work. Clean abandoned temporary visual artifacts after crash/recovery where possible.

A future `retain visual captures` mode is separate opt-in and must warn that retained images can contain arbitrary sensitive information.

### Re-analysis guarantee

CurioTrace guarantees re-analysis from the durable observations it intentionally preserves. It does not promise future OCR/Vision reruns against historical pixels under the default privacy-preserving screenshot policy.

### Deletion

Deleting a session removes all CurioTrace-managed source, derived, optional retained visual data, generated managed artifacts, and session temporary files. CurioTrace cannot revoke copies deliberately exported elsewhere or guarantee immediate deletion from independent OS/filesystem backups.

### Local storage protection

Persist session data in application-private storage with normal OS access protections. Use platform-appropriate secret/key facilities and encryption at rest for the main persisted observation store where maintainable. Logs/crash diagnostics do not duplicate browsing text/screenshots by default.

CurioTrace does not claim protection against a fully compromised OS/user account.

### Storage pressure

Expose CurioTrace-managed storage usage. Durable source sessions are not silently aged out merely because an implementation threshold is reached; automatic cleanup of durable data requires an explicit user policy. Transient visual cleanup is automatic by design.

## 10. Markdown and LLM generation

LLM processing is a replaceable downstream layer.

### Deterministic Markdown — required

CurioTrace must generate a basic structured Markdown document locally without any model/API/network dependency. It should represent the session metadata, navigation, observations, exposure/revisit information when available, interactions, gaps/errors/uncertainty, and provenance identifiers.

It may be verbose; reliability/portability take precedence over polished prose.

### LLM-organized Markdown — optional enhancement

An LLM may reorganize/compress the same single-session evidence into a clearer document. It may group related observed material, summarize viewed content, prioritize long/repeated exposure or explicit interactions, and improve timeline/topic structure.

It must not silently add cross-session information, invent unobserved content, assert unobservable user state as fact, erase known uncertainty/gaps, or destroy practical provenance.

### Provider independence

Use a provider-independent conceptual adapter. Supported implementations may later include local models, BYOK providers/gateways, or a future bundled hosted service. No hosted provider is required for core recording/export.

### External transfer

External OCR/Vision/LLM is off by default. Authorization is separate from recording and granted by provider plus data class/capability. Distinguish at minimum text/derived metadata, OCR text, and visual screenshots/frames.

Repeated confirmation is not required for every export if the user has already authorized the same provider/data scope. Expanding to a new data class requires new explicit authorization.

Raw/retained screenshots are never included in external requests by default; external Vision requires separate visual-data authorization.

The target-session boundary remains strict: the generator does not automatically search prior sessions, personal knowledge stores, or unrelated browser history.

### Provenance and validation

Generated artifacts record conceptually: session ID, source/export schema version, generator version, whether LLM organization was used, provider/model identifier when applicable, and source references for important content where practical.

Free-form LLM output is never source data. Mechanically validate checkable claims such as observation IDs, URLs/page identity, timestamps/counts/durations, navigation order, and quoted/selected text where feasible.

If LLM generation fails/interrupts/does not validate, preserve the session and fall back to deterministic Markdown rather than fabricating success.

External-provider retention/training/legal handling is outside CurioTrace after authorized transmission; the chosen provider and applicable external responsibility must be clear to the user.

## 11. Completion criteria

### Basic functionality

After a real browsing session, the user can approximately reconstruct:

> what I saw, what I looked at for longer, and where I returned.

### Measurement quality

Across different sites/display formats, visible content is captured with enough practical stability to track the same information through time. Perfect full-document reconstruction is not required.

### Markdown quality

A downstream LLM given only the exported Markdown can recover the important shape/content of the recorded session without major omission or fabricated observations.

### Privacy

Configured/excluded information does not enter durable content storage or external processing under the defined guarantees. Known technical limitations are stated explicitly rather than hidden behind stronger claims.

### Re-analysis

Future analyzers/Markdown generators can reuse retained source observations to the extent guaranteed by the data-lifecycle policy.

## 12. Milestones

### M1 — Minimal tracking

Chrome/Edge: explicit Start/Stop, navigation, minimum viable content capture, privacy-safe local persistence, and inspection of a recorded session.

### M2 — Exposure reconstruction

Recognize continued display, redisplay/revisit, exposure duration, selection/copy linkage, and source provenance.

### M3 — Session Viewer

Human-facing reconstruction of what was seen, long/repeated exposure, interactions, gaps, and provenance. Use this milestone to compare system reconstruction against real browsing experience.

### M4 — Firefox

Bring core session semantics to Firefox with documented platform limitations and a compatible shared data model.

### M5 — Export

Machine-readable export plus provenance-preserving deterministic Markdown and optional LLM organization.

### M6 — External handoff

Low-friction transfer of a completed single-session artifact to downstream LLM/Wiki/code-agent workflows without moving cross-session knowledge management into CurioTrace.

## 13. Implementation freedom

The following are candidates, not fixed requirements:

- screenshot capture + local OCR;
- DOM/viewport extraction;
- hybrid DOM + screenshot/OCR;
- perceptual hash / screenshot diff;
- fuzzy OCR-text matching;
- browser extension + native helper;
- local LLM/OCR;
- BYOK external OCR/Vision/LLM;
- specific language/framework/database/native runtime.

A simpler, safer, or more accurate method may replace any candidate if it preserves this specification.

Capture architecture is intentionally deferred to #8, where alternatives must be tested against real web surfaces, privacy behavior, portability, fidelity, and resource cost.
