# CurioTrace Evaluation Contract

Status: initial QA contract for issue #15. Numerical soft targets should be calibrated with the first working recorder, but hard safety/correctness gates are already normative.

## 1. Purpose

CurioTrace intentionally discards high-fidelity page/screenshot intermediates after successful session finalization. Evaluation therefore has to detect information loss, unsupported reconstruction, privacy failures, and provenance breakage before a capture or Markdown-generation implementation is considered safe to rely on.

The same versioned golden sessions should compare:

- capture implementations;
- content-identity algorithms;
- exposure reconstruction;
- Session Viewer behavior;
- deterministic fallback Markdown;
- frontier-model `session.md` generation;
- browser/platform variants.

Use repository-owned synthetic fixtures wherever possible so tests do not depend on third-party copyrighted/private content.

## 2. Ground-truth model

Each golden session defines explicit expected evidence rather than an expected prose summary.

A scenario manifest should be able to describe at least:

- ordered navigation/view segments;
- content units that are visible in each viewport state;
- content units that are deliberately never visible;
- expected redisplay/revisit relationships;
- timing windows or exposure ranges;
- selection/copy events;
- source URL/title identity;
- expected capture gaps/unsupported surfaces;
- privacy-forbidden canary values/regions;
- exact excerpts whose wording/structure is intentionally important;
- content that may only be paraphrased in the durable artifact.

Do not encode inferred mental state (interest, understanding, agreement) as ground truth.

## 3. Core fixture scenarios

The initial suite should contain repository-owned local fixtures for:

1. ordinary article scrolling;
2. scroll away and back to an earlier paragraph;
3. leave a page and later revisit it;
4. code/GitHub-like page with code blocks;
5. table + equation/layout-sensitive content;
6. canvas/image-only text mixed with ordinary DOM;
7. SPA route/content replacement without full reload;
8. infinite-scroll append/remove behavior;
9. same-origin iframe;
10. cross-origin iframe;
11. sandboxed/nested iframe;
12. editable text, password, search, payment-like and contenteditable controls filled with privacy canaries;
13. explicitly excluded page/domain containing privacy canaries;
14. tab switching/background/minimize/restore;
15. selection/copy on ordinary non-editable content;
16. built-in PDF/restricted-surface behavior using a non-sensitive repository-owned PDF fixture;
17. permission denial before Start;
18. Pause and interrupted-session recovery;
19. summarizer failure / grace-window expiry / trace-only fallback;
20. URL that later returns changed content, to verify later re-fetch is never attributed to the historical observation.

## 4. Evaluation layers

### A. Capture evidence

Measure whether expected visible content units were observed.

Mechanical metrics where possible:

- visible-content-unit recall;
- false-observation rate for units known never to be visible;
- source URL/title correctness;
- expected capture-gap reporting;
- selected/copied text integrity.

Capturing a whole hidden/full document does **not** count as better recall. A unit that was never visible but is recorded as observed is a false positive.

### B. Content identity

Golden manifests label selected pairs/groups as:

- same substantive content;
- different content;
- intentionally ambiguous.

Measure pair/group precision and recall. Ambiguous cases should remain uncertain rather than being force-classified for score.

### C. Exposure / revisit

Evaluate:

- continuous interval boundaries;
- redisplay count;
- revisit count;
- exclusion of Pause/background/minimize/lock intervals where observable;
- duration error within the measurement precision supported by the capture schedule.

Do not optimize toward apparent precision beyond the recorder's sampling/event evidence.

### D. Privacy negative tests — hard gate

Fixtures contain unique canaries in:

- password fields;
- ordinary editable form values;
- excluded pages;
- excluded/sensitive frame regions;
- optional private-context fixtures where test automation safely supports them.

Search all CurioTrace-managed outputs reachable to the test harness:

- durable observation store;
- derived/cache data;
- logs/diagnostics;
- persisted PoC/test images;
- `session.md`;
- MCP payload fixtures / mocked external requests.

**Required result: zero forbidden canary occurrences. One confirmed leak is a test failure, not an averaged metric.**

Also verify that denied broad host permission results in no recording/capture session.

### E. Copyright/source-retention policy — hard structural gate

For repository-owned article fixtures, verify the product architecture follows the same rules intended for third-party works:

- no durable full-page screenshot by default;
- no durable near-complete extracted/OCR page copy by default;
- full/high-fidelity intermediates are deleted after successful finalization;
- durable output contains source pointers + compact observed semantics;
- exact quotations are limited to fixture spans explicitly marked as exact evidence/selection/structure-sensitive content;
- downstream re-fetch is represented as a separate source acquisition.

Do not define a fake universal legal word/percentage threshold. Test conformance to the product retention policy instead.

### F. Markdown semantic compression

Each scenario labels atomic semantic facts/content units supported by the historical observation evidence.

Score `session.md` for:

- semantic coverage of observed material;
- unsupported-content/hallucination rate;
- source/page coverage;
- chronology/navigation correctness;
- exposure/revisit/selection evidence preservation;
- provenance integrity;
- explicit uncertainty/gap preservation;
- exact-evidence fidelity where exact wording was required;
- absence of unobservable user-state claims.

A frontier model may reorganize content; it need not reproduce one canonical wording.

### G. LLM Wiki ingestibility

Give the finalized `session.md` to an ingest agent **without CurioTrace internals** and verify that it can:

- identify the session and time range;
- identify source URLs/titles;
- distinguish what CurioTrace observed from later source pointers;
- identify important semantic content without needing the original page mirror;
- preserve gaps/uncertainty;
- ingest the artifact as one immutable source rather than mistaking it for a finished Wiki page.

## 5. Hard correctness gates

The following are release-blocking regardless of average score:

- any confirmed privacy-forbidden canary persisted or externally transmitted;
- capture occurs after a required host-permission denial;
- capture occurs in `IDLE`/`PAUSED` contrary to the lifecycle contract;
- excluded page content is durably stored where the privacy contract forbids it;
- an exact quote is fabricated or materially altered while labelled exact;
- later-refetched content is represented as historically observed;
- generated Markdown asserts unsupported user mental state as observation;
- finalized session loses required source identity/provenance needed to interpret its content;
- high-fidelity temporary material survives successful finalization contrary to lifecycle policy.

## 6. Initial quality targets to calibrate

These are starting engineering targets, not immutable product promises. Calibrate them against the first working M1/M2 recorder and document any revision.

- All navigated non-excluded source/view segments should appear in the machine trace, including segments with little exposure.
- Materially observed semantic content should target at least **95% weighted coverage** in frontier-model `session.md` on synthetic golden sessions.
- Unsupported semantic statements should target **0%**; any non-zero cases require review even if an aggregate score remains high.
- Source URL/title/visit-order/provenance fields should target **100% mechanical validity** wherever the source data exists.
- Exposure durations should be judged against an explicit tolerance derived from actual capture debounce/scheduling, not a fixed arbitrary millisecond target.

For semantic coverage weighting, use the **scenario's test annotation**, not a claim that exposure time equals user interest. Golden authors may mark content as material because the scenario was constructed to test it.

## 7. Frontier-model comparison

Do not hard-code a vendor/model requirement. For each candidate summarizer record:

- model/provider identifier and date/version when available;
- prompt/Skill version;
- input schema version;
- semantic coverage;
- unsupported claims;
- exact-evidence fidelity;
- provenance/schema validation outcome;
- generation failure rate;
- representative cost/latency where relevant.

A model is acceptable only if it passes hard gates and reaches the calibrated semantic-quality bar across the golden suite.

## 8. Versioning and reproducibility

Golden scenarios, expected manifests, capture implementation, analyzers, Markdown schema, prompts/Skills, and evaluation code are versioned independently.

When an expected result changes because the product semantics changed, update the scenario expectation with an explicit decision reference rather than silently moving the test target.

Keep synthetic fixture contents deterministic. External/live websites may be used for exploratory robustness testing, but not as the only reproducible golden oracle.

## 9. CI strategy

Split tests so cheap deterministic checks run on every relevant change and expensive/browser/model tests run only when needed.

Parallelize independent deterministic tests within available CI cores. Candidate layers:

- schema/unit tests;
- privacy canary scans;
- fixture/parser/content-identity tests;
- browser integration matrix;
- model evaluation jobs.

Do not send real browsing-history fixtures to CI or external model providers. Public CI uses repository-owned synthetic data only.
