# CurioTrace Golden Sessions

This directory contains repository-owned synthetic browsing scenarios for the evaluation contract in `docs/EVALUATION.md`.

The suite describes **observable evidence**, not a canonical prose summary and not inferred user mental state.

## Layout

Each scenario lives under `tests/golden/scenarios/<id>/` and contains:

- `scenario.json` — machine-readable expected navigation/visibility/privacy/protocol evidence;
- `fixture/` — deterministic local web/PDF content when the scenario needs its own fixture;
- optional notes or helper scripts.

The schema is `tests/golden/schema/scenario.schema.json`.

## Principles

- Use unique stable content-unit IDs in fixture markup (for example `data-golden-id`).
- Put privacy canaries only in synthetic fixtures. Never use real secrets.
- A canary marked forbidden must occur zero times in durable outputs/logs/mock external payloads.
- `must_observe` means the scenario intentionally exposes that unit; `must_not_observe` means recording it as visible is a false positive.
- Timing expectations are ranges/tolerances, not invented exact precision.
- Later source re-fetch is a different acquisition and must never satisfy a historical `must_observe` expectation.
- Permission/helper/capture-authority failures are part of correctness and may define an expected final state even when no semantic content should be observed.
- Scenario changes that alter semantics should cite the product/design decision that changed the expected behavior.

## Current scenarios

- `ordinary-scroll` — viewport-only capture and scroll-away/redisplay behavior.
- `privacy-redaction` — editable/password/iframe canaries must never leak while ordinary visible text remains observable.
- `permission-denial` — required host-permission refusal leaves CurioTrace `IDLE` and creates no partial recording.
- `helper-disconnect` — loss of native-helper authority stops capture, rejects old-epoch work, and restarts as `INTERRUPTED`.
- `fingerprint-only-pdf` — repository-owned PDF canary remains visible to the user but must not become OCR/semantic output in Tier-2 fingerprint-only mode.

Additional scenarios from `docs/EVALUATION.md` should be added incrementally as M1/M2 capabilities become executable.

## CI validation

`tests/test_golden_scenarios.py` performs zero-dependency structural checks over every scenario and validates key hard-gate assumptions. It also sanity-checks the synthetic PDF fixture's `startxref` location.

Real browser execution remains a separate integration layer; a valid manifest is not evidence that browser capture behavior passed.