# CurioTrace Product Specification

Status: product semantics stabilized through design issues #1–#7 and #18–#20. Capture architecture remains intentionally open until #8.

This document defines user-visible value, responsibility boundaries, privacy/copyright posture, lifecycle semantics, output contract, and completion criteria. Internal implementation remains replaceable unless a choice is required to preserve those semantics.

## 1. Product goal

CurioTrace records web browsing during a session that the user explicitly starts so the session can later be reconstructed and reused without requiring manual note-taking while browsing.

The system should make it possible to recover, as observations rather than guesses:

- which pages were visited;
- which information was actually visible;
- how long information remained exposed;
- where information was shown again or revisited;
- which allowed page content was selected/copied;
- how the user moved between pages;
- when the session started, paused/resumed, ended, or was interrupted.

The primary durable human/LLM-facing artifact is a Markdown representation of a **single browsing session**. It is intended to become an immutable source document for a downstream LLM Wiki or similar system.

CurioTrace also retains/exports machine-readable session data sufficient for local audit and re-analysis within the guarantees of the data-lifecycle policy.

## 2. Responsibility boundary

CurioTrace owns the pipeline from explicit session recording through single-session export:

1. capture allowed observable browsing events/content;
2. maintain privacy/copyright-safe temporary source material;
3. persist compact source observations locally;
4. derive session-local content identity/exposure/revisit structure;
5. let a human inspect the reconstructed session through a CLI/text-first surface, with richer GUI optional rather than required;
6. generate or obtain a validated single-session `session.md`;
7. export machine-readable session data;
8. hand the finalized artifact to downstream tools.

CurioTrace does **not** own:

- searching for related past sessions;
- cross-session knowledge synthesis;
- long-term Wiki/knowledge-graph maintenance;
- persistent interest/profile modeling;
- cross-document entity linking as a long-term knowledge base;
- deciding what the user believes, understands, remembers, agrees with, intends, or should do.

Those tasks belong to downstream systems such as an LLM Wiki, Codex/Claude-style agents, or other knowledge-management tools.

## 3. Observation and inference

CurioTrace separates three epistemic layers.

### Observed

Directly recorded facts/events, for example:

- content was visible at a time;
- a tab/view/navigation changed;
- a selection/copy event occurred;
- capture failed;
- tracking was paused.

### Derived

Algorithmic results computed from observations, for example:

- two observations probably represent the same content;
- an exposure interval lasted approximately a given duration;
- content was redisplayed/revisited;
- normalized text or matching fingerprints.

Derived records preserve provenance to observations and express uncertainty where relevant.

### Generated / semantic

Human/LLM-readable semantic compression and organization of the single session.

Generated text is not promoted into the source-observation log. CurioTrace may summarize what was observed, but must not assert unobservable user state as fact. In particular it must not claim that the user understood, agreed, solved, remembered, intended, or cared about something merely from browsing behavior.

## 4. Session lifecycle

Application/process state is separate from tracking authorization. A browser extension or helper may remain enabled or auto-start, but that alone does not authorize browsing-content capture.

### Recording states

`IDLE -> RECORDING <-> PAUSED -> FINISHED`

Exceptional transition:

`RECORDING/PAUSED -> INTERRUPTED`

Recovery:

`INTERRUPTED -> explicit Resume -> RECORDING`

`INTERRUPTED -> Stop -> FINISHED`

### Rules

- Tracking begins only through explicit `Start`.
- `Start` is zero-friction; no mandatory title/purpose form blocks recording.
- `Pause` preserves session identity but stops browsing-content acquisition.
- `Stop` ends recording and begins finalization/summarization.
- Before `Start`, while `IDLE`, or while `PAUSED`, browsing URL/content/DOM/screenshot-equivalent observations are not captured.
- Browser closure/crash does not necessarily end the session if the recorder remains alive.
- OS restart or recorder/helper loss never silently restores `RECORDING`; the unfinished session becomes `INTERRUPTED`.
- OS lock/suspend is not browsing exposure and stops content capture.
- Primary Start/Pause/Resume/Stop controls must be reachable from the browsing context without opening a full management UI.
- The browser control surface is intentionally minimal: explicit lifecycle control, permission consent, and readily inspectable recording/interrupted/helper state.
- Management, diagnostics, export, deletion/cleanup, helper/storage inspection, and advanced integration controls should prefer CLI/text interfaces unless a concrete browser-UI requirement justifies otherwise.

### Post-Stop summarization states

A stopped session that still needs frontier-model summarization may enter:

`PENDING_SUMMARY -> GENERATING -> VALIDATING -> FINALIZED`

Failure may return to `PENDING_SUMMARY` while the finite retry window remains. Expiry without successful generation produces `FALLBACK_FINALIZED` and deletes high-fidelity temporary source material.

## 5. Browser support

Required targets:

- Chrome;
- Edge;
- Firefox, as a formal parity target after the initial Chrome/Edge implementation stabilizes.

Browser-specific limitations may exist, but downstream session data should share one semantic model rather than separate browser-specific product models.

## 6. Source observation model

The durable session history is a logically append-only sequence of privacy-filtered compact observations.

The semantic model must represent at least:

- session-state transitions;
- browser/tab/view navigation and activation;
- visibility state needed for exposure reconstruction;
- content observations and capture method/status;
- allowed selection/copy interactions;
- privacy allow/block/mask decisions without leaking excluded content;
- capture/OCR/parser failures and unsupported/gap states.

Every durable observation has stable session/event identity. Runtime browser/tab IDs are not assumed globally stable across restarts; when continuity cannot be established reliably, CurioTrace creates a new identity rather than guessing.

Persist both human chronology and duration-safe timing: wall-clock timestamps plus a monotonic/session-relative clock or equivalent. Exposure duration must not depend only on mutable wall-clock differences.

Near-complete OCR/DOM/page text is **not** the default durable source record. It may exist transiently during capture and Markdown generation. Durable observations retain compact content evidence sufficient for session reconstruction, plus source pointers/provenance.

Derived records carry analyzer/schema version and provenance. Later re-analysis produces new derived results without rewriting historical source meaning.

Known missing data is represented explicitly rather than treated as evidence that nothing changed or nothing was seen.

## 7. Content identity and exposure

Content identity is derived, not directly observed.

Analyzers may combine text, viewport geometry, visual fingerprints, DOM/accessibility hints, page identity, and temporal continuity. The specific algorithm is replaceable.

### Same content

Observations may be grouped as the same substantive information despite scrolling, resize, wrapping, rerendering, OCR noise/segmentation, or reload. Material content changes should produce a new/revised identity. Ambiguous matches remain ambiguous rather than being forced together.

### Continuous exposure

A maximal interval in which the same content is considered visible without evidence of a meaningful non-visible break. Known Pause/lock/background/minimize/navigation-away intervals end exposure. CurioTrace must not claim timing precision finer than its observations support.

### Redisplay

Previously visible content becomes non-visible, then visible again without necessarily leaving the surrounding page context.

### Revisit

The user leaves the relevant page/view context and later returns to the same page/content context in a later navigation/view segment.

### Visibility caveat

Hidden/background tabs, minimized known-nonvisible browser surfaces, OS lock/suspend, Pause/Idle, and navigation-away periods are excluded from exposure. Focus loss alone is not automatically proof of invisibility because a window may remain visible side-by-side. Unknown occlusion remains an explicit limitation.

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
- are not written to normal durable storage/logs/crash reports/telemetry by default;
- are locally redacted before OCR/persistence/external transfer where identifiable sensitive regions are known;
- require separate opt-in if a future mode intentionally retains them.

### Navigation / embedded content

Re-evaluate eligibility on navigation, redirects, reload, and observable SPA route changes. Pending capture for a newly excluded view should be discarded before persistence.

Known excluded embedded frames/regions must be masked/omitted where technically possible. If a configured exclusion cannot be reliably masked, visual capture of the affected view fails closed rather than knowingly persisting the excluded region.

### Explicit limitation

CurioTrace cannot reliably determine whether arbitrary allowed visible content is confidential. Ordinary text, images, PDFs, canvas/code/chat content, and other unmarked pixels may contain secrets indistinguishable from ordinary content.

The product promise is therefore about capture scope, explicit exclusions, browser-identifiable sensitive inputs, local redaction, retention, and transfer boundaries — not perfect semantic secret detection.

## 9. Copyright and third-party source-content boundary

CurioTrace is a browsing-trace system, not a durable full-page archiver or redistribution system.

### Full expressive content is transient by default

During recording and session generation, CurioTrace may temporarily hold screenshots, OCR text, DOM-extracted text, or other near-complete source content needed to reconstruct the session and generate `session.md`.

After `session.md` and the compact durable observation record have been successfully generated and validated, unnecessary full-content intermediates are deleted automatically.

There is no default core feature whose purpose is to save a complete page/article for later rereading.

### Durable source record

Persist, where available:

- source/canonical URL;
- title;
- observation time/order and navigation provenance;
- exposure/re-display/revisit/interaction evidence;
- concise semantic descriptions of the portions actually observed;
- capture uncertainty/failures;
- short exact excerpts only where wording/structure is materially important or where the user explicitly selected/copied text.

A URL alone is insufficient because the historical page may later change, disappear, personalize differently, or become inaccessible.

Code, tables, equations, quotations, and other structure-sensitive material may require more exact preservation than ordinary prose, but this exception must not become a default full-work mirror.

### Access controls

CurioTrace must not bypass paywalls, authentication, DRM, anti-copy controls, or other restrictions to obtain content unavailable to the user through the normal rendered browsing session.

### Downstream re-fetch

A downstream LLM Wiki/Codex/Claude ingest workflow may independently fetch a recorded URL when permitted. That later fetch is a new source acquisition and must not be represented as content the user historically observed.

### Distribution boundary

The default product posture is local, user-controlled personal knowledge use. CurioTrace does not claim blanket copyright clearance for business use, redistribution, team sharing, publication, or hosted third-party-content archiving.

Future hosted capture, collaborative archives, or redistribution features require a new design/legal review rather than inheriting this decision.

## 10. Data lifecycle

CurioTrace distinguishes durable compact observations from high-fidelity temporary source material.

### Durable compact observations

Retain locally by default until user deletion or an explicitly configured cleanup policy:

- allowed session/navigation events;
- source URL/title metadata where allowed;
- compact observed-content units sufficient for session reconstruction;
- source geometry/timing/visibility metadata;
- exposure/revisit/interaction evidence;
- short justified exact excerpts/selections;
- capture/error metadata needed to interpret gaps.

### Recomputable derived data

Normalized/search representations, content identity, exposure intervals, fingerprints/features, and generated analysis metadata are versioned derived/cache data. They may be discarded/rebuilt from retained compact observations to the extent those observations support.

### High-fidelity temporary data

Raw/unredacted screenshots, raster frames, and near-complete OCR/DOM/page text are ephemeral by default.

They may remain available during an active session and through the bounded post-Stop summarization/validation window. They are deleted immediately after successful finalization or when the safe pending deadline expires.

A future intentional visual/full-source retention mode would require a separate explicit opt-in and corresponding privacy/copyright review.

### Re-analysis guarantee

CurioTrace guarantees future session-local analysis and Markdown regeneration only from the compact durable observations it intentionally preserves. It does not promise complete reconstruction of historical page text or future OCR/Vision reruns against discarded pixels.

This is an explicit privacy/copyright-over-maximum-reprocessability trade-off.

### Deletion

Deleting a session removes all CurioTrace-managed source, derived, optional retained visual data, generated managed artifacts, and session temporary files. CurioTrace cannot revoke copies deliberately exported elsewhere or guarantee immediate deletion from independent OS/filesystem backups.

### Local storage protection

Persist durable session data in application-private storage with normal OS access protections. Use platform-appropriate secret/key facilities and encryption at rest where maintainable. Logs/crash diagnostics do not duplicate browsing text/screenshots by default.

Short-lived pending high-fidelity data, if it must survive a process restart, remains in application-private temporary storage and remains subject to the same expiry/cleanup deadline.

CurioTrace does not claim protection against a fully compromised OS/user account.

## 11. `session.md`, LLM Wiki, and external summarization

### Downstream role

Inside CurioTrace, `session.md` is a derived artifact. Once handed to the downstream LLM Wiki, it becomes an immutable source document in that system's `raw/` layer.

It is **not**:

- a finished Wiki page;
- a cross-session synthesis;
- a long-term concept/entity graph;
- a replacement for original web pages;
- a claim about the user's understanding/beliefs.

### Required semantic structure

A normal `session.md` contains conceptually:

- stable artifact/session/schema metadata;
- a compact session overview;
- a navigation timeline;
- stable per-artifact source IDs (`S1`, `S2`, ...);
- URL/title/time/order for each observed source;
- compact paraphrase of the portions actually observed;
- exposure/revisit/interaction evidence;
- narrowly justified exact evidence;
- gaps/uncertainty/capture failures;
- practical provenance references.

Observed facts, derived paraphrase, verbatim evidence, and uncertainty remain distinguishable.

The artifact is optimized for ingestibility and evidence, not minimum token count or polished Wiki prose.

### No silent web enrichment

The CurioTrace summarizer generates the historical artifact from CurioTrace-observed evidence only. It must not follow recorded URLs and silently add newly fetched information into `session.md`.

Downstream LLM Wiki ingestion may fetch URLs independently as separate sources.

### External frontier-model summarization

High-quality semantic compression is a product-critical task because full/high-fidelity temporary source content is normally discarded after successful generation.

CurioTrace therefore does **not** bundle a heavyweight local LLM by default. It delegates high-quality Markdown generation to external frontier-model agents.

- MCP is the canonical machine integration boundary.
- Optional host-specific Skills tell Codex, Claude Code, or similar agents how to perform the workflow correctly.
- Skills are instructions, not the security/data-access boundary.
- Provider credentials/billing remain with the external agent host when possible.
- Recording works without any external model connected.

### Authorization

Recording authorization does not imply permission to disclose session content to an external model.

At minimum support:

- explicit first-time authorization of a named integration/agent host;
- clear disclosure that observed content may be transmitted to that host/provider for summarization;
- revocation;
- no external disclosure when no integration is authorized.

A user may optionally grant a trusted integration permission to auto-summarize newly stopped sessions without a confirmation dialog for every session.

### Capability limitation

An authorized summarizer gets narrow access to one pending session, not arbitrary filesystem/database access. Conceptually it can inspect the session manifest, read authorized temporary content/observation metadata, submit a candidate `session.md`, receive validation errors, and resubmit.

Only CurioTrace finalizes the artifact and deletes temporary source material.

### Validation

Before deleting high-fidelity source input, CurioTrace validates at least:

- expected materially observed sources are represented;
- source IDs/URLs/provenance references are internally consistent;
- important selection/copy and high-exposure/revisit events are not silently omitted;
- no unsupported claims about user mental state are introduced;
- the output has the expected artifact structure and records known gaps/uncertainty.

Runtime validation should rely on deterministic checks where feasible. Semantic quality is benchmarked through golden sessions against representative frontier models; a second heavyweight model is not required on every run unless later evidence justifies it.

### Pending-source deadline and fallback

High-fidelity/full-text source material is never retained indefinitely waiting for an external agent.

Initial product target:

- finite post-session grace window, initially about **1 hour**;
- immediate deletion after successful validation/finalization;
- optional one-time user-visible extension before expiry;
- expiry produces a deterministic **trace-only fallback Markdown** and deletes high-fidelity temporary material.

The exact default grace duration may be tuned by later usability/security testing but must remain finite and visible.

The fallback artifact is explicitly degraded and need only preserve deterministically available facts such as session/time metadata, URLs/titles/navigation order, exposure/revisit/interaction facts, allowed explicit excerpts/selections, and a clear notice that semantic summarization is incomplete.

### Immutability after handoff

After a generated artifact is handed off as an LLM Wiki source, it is not silently rewritten. A later improved regeneration produces a new artifact/version.

## 12. Completion criteria

### Basic functionality

After a real browsing session, the user can approximately reconstruct:

> what I saw, what I looked at for longer, and where I returned.

### Measurement quality

Across different sites/display formats, visible content is captured with enough practical stability to track the same information through time. Perfect full-document reconstruction is not required.

### Markdown quality

A downstream LLM given only the finalized normal `session.md` can recover the important shape/content of the recorded session without major omission or fabricated observations.

Evaluation must cover semantic coverage, omission/hallucination rate, provenance, important exact/structured evidence, exposure/revisit/selection preservation, and LLM Wiki ingestibility.

### Privacy

Configured/excluded information does not enter durable content storage or external processing under the defined guarantees. Known technical limitations are stated explicitly rather than hidden behind stronger claims.

### Copyright/source retention

The default product does not durably mirror complete third-party works merely because they were visible during a session. High-fidelity/full-text source material is transient through generation/validation and is deleted according to the lifecycle contract.

### Re-analysis

Future analyzers/Markdown generators can reuse the compact retained source observations to the extent guaranteed by the data-lifecycle policy. Complete historical source reconstruction is intentionally not guaranteed.

## 13. Milestones

### M1 — Minimal tracking

Chrome/Edge: explicit Start/Stop, navigation, minimum viable content capture, privacy-safe local persistence, and inspection of a recorded session.

### M2 — Exposure reconstruction

Recognize continued display, redisplay/revisit, exposure duration, selection/copy linkage, and source provenance.

### M3 — Session reconstruction and inspection

Provide a human-inspectable reconstruction of what was seen, long/repeated exposure, interactions, gaps, and provenance. The baseline surface may be CLI/text/Markdown; a polished Session Viewer is optional and should be built only if later evidence shows that a richer GUI materially improves a concrete workflow.

### M4 — Firefox

Bring core session semantics to Firefox with documented platform limitations and a compatible shared data model.

### M5 — Markdown Export

Produce the #19-compliant normal `session.md` through the external frontier-model workflow, plus the deterministic trace-only fallback and machine-readable export. Validate before deletion of high-fidelity temporary source material.

### M6 — External handoff

Low-friction handoff of finalized `session.md` to LLM Wiki/Codex/Claude-style workflows without moving cross-session knowledge management into CurioTrace.

## 14. Implementation freedom

The following remain candidates unless constrained by the specification:

- screenshot capture + local OCR;
- DOM/viewport extraction;
- hybrid DOM + screenshot/OCR;
- perceptual hash / screenshot diff;
- fuzzy OCR-text matching;
- browser extension + native helper;
- specific language/framework/database/native runtime;
- exact MCP tool/resource names and local transport;
- exact Skill packaging;
- exact post-Stop grace-window duration;
- specific external frontier model/provider.

A simpler, safer, or more accurate method may replace any candidate if it preserves this specification.

Capture architecture is intentionally deferred to #8, where alternatives must be tested against real web surfaces, privacy behavior, copyright/source-retention behavior, browser portability, fidelity, and resource cost.
