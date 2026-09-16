# CurioTrace Decision Log

This file records stable product-level decisions. Detailed discussion remains in the linked GitHub issues. Implementation choices that remain replaceable should not be promoted here unless they affect product semantics.

## 2026-09-16 — D001: Single-session responsibility boundary

Source: #1

CurioTrace owns explicit session observation, local persistence, session-local reconstruction, human inspection, machine-readable export, and single-session Markdown generation.

Cross-session synthesis, long-term Wiki/knowledge maintenance, persistent interest/profile modeling, and conclusions about the user's internal cognitive state are outside CurioTrace core.

Observation, derived analysis, and generated semantic organization remain separate layers. Optional downstream failures must not invalidate already captured source observations.

## 2026-09-16 — D002: Explicit tracking lifecycle

Source: #2

Process/app presence is not tracking authorization. Tracking starts only through explicit `Start`; `Pause` stops browsing-content acquisition while preserving session identity; `Stop` finalizes the session.

State model:

`IDLE -> RECORDING <-> PAUSED -> FINISHED`

Unexpected recorder/process loss produces `INTERRUPTED`, which requires explicit Resume/Stop. OS lock/suspend is not exposure. A surviving recorder may continue the same state after wake.

Normal controls must be reachable from the browsing context without requiring a full management UI.

## 2026-09-16 — D003: LLM is replaceable downstream processing

Source: #3

Recording, persistence, machine-readable export, and baseline deterministic Markdown work without an LLM/provider/network connection.

LLM organization is an optional enhancement for a single session. Provider integrations are adapter-based and external transfer is off by default. Authorization is scoped by provider and data class/capability. Visual data requires separate authorization.

Generated model output never becomes source observation data and mechanically checkable claims should be validated where feasible. Failure falls back to deterministic Markdown.

## 2026-09-16 — D004: Strict privacy gate and explicit limitation

Source: #4

Privacy filtering occurs before content persistence as early as technically feasible. Private/incognito contexts are excluded by default. Users can Pause, exclude the current page/view, exclude the current tab, and persistently exclude a site/domain.

Excluded pages do not persist URL/title/content by default. Editable form values and global clipboard contents are not observation content. Known sensitive/editable regions are redacted before OCR/persistence/external transfer where technically possible.

Unredacted screenshots may transiently exist in process memory only when platform capture APIs make this unavoidable; they are not durably stored/logged/externalized by default.

CurioTrace does not claim perfect semantic secret detection. Arbitrary sensitive information visibly rendered inside an allowed region may be captured because it can be indistinguishable from ordinary content.

## 2026-09-16 — D005: Durable text/events, ephemeral raw pixels

Source: #5

Privacy-filtered source events/text/timing/geometry are retained locally by default until user deletion or an explicitly configured cleanup policy.

Derived normalized text, content identity, exposure, fingerprints/features, and generated analysis metadata are versioned and recomputable.

Raw/unredacted screenshots/raster frames are ephemeral by default. Future intentional visual retention is a separate explicit opt-in.

The re-analysis guarantee applies to retained source observations, not to rerunning future OCR/Vision against historical pixels.

Session deletion removes CurioTrace-managed copies but cannot revoke deliberately exported copies or independent filesystem backups.

## 2026-09-16 — D006: Append-only source observations with provenance

Source: #6

A session's durable source history is logically append-only for analysis. Source events include lifecycle, navigation, visibility, content observations, allowed interactions, privacy decisions, and capture/error/gap status.

Every source event has stable event/session identity. Wall-clock chronology and duration-safe monotonic/session-relative time are both retained conceptually.

Raw retained text is distinct from normalized/search text. Derived records carry analyzer/schema version and references to supporting source observations. Generated Markdown provenance remains transitively resolvable to source observations.

Deletion/privacy redaction and meaning-preserving storage migrations are explicit exceptions to logical immutability.

## 2026-09-16 — D007: Exposure is evidence-based, not an interest score

Source: #7

Content identity, continuous exposure, redisplay, and revisit are derived concepts with versioned algorithms and provenance.

Repeated sampling alone must not create false revisits/redisplays. Known Pause/lock/background/minimize/navigation-away intervals do not count as exposure. Unknown occlusion remains uncertainty rather than fabricated certainty.

Selection/copy attaches to the best-supported content/source observation when allowed; ambiguous targets remain unresolved/page-level.

A future salience score may exist only as a decomposable convenience view. Observable durations, repeats, and interactions remain primary; the score is not a direct measurement of user interest, understanding, or importance.

## Pending architecture decision

#8 will empirically compare capture architectures (DOM/viewport extraction, screenshot + local OCR, hybrid approaches, browser extension + native helper, etc.) against real web surfaces, privacy constraints, browser portability, fidelity, and resource cost.

Until #8 is resolved, these mechanisms remain candidates rather than product requirements.
