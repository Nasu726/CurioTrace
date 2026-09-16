# CurioTrace Golden Sessions

This directory contains repository-owned synthetic browsing scenarios for the evaluation contract in `docs/EVALUATION.md`.

The suite describes **observable evidence**, not a canonical prose summary and not inferred user mental state.

## Layout

Each scenario lives under `tests/golden/scenarios/<id>/` and contains:

- `scenario.json` — machine-readable expected navigation/visibility/privacy evidence;
- `fixture/` — deterministic local web content when the scenario needs its own fixture;
- optional notes or helper scripts.

The schema is `tests/golden/schema/scenario.schema.json`.

## Principles

- Use unique stable content-unit IDs in fixture markup (for example `data-golden-id`).
- Put privacy canaries only in synthetic fixtures. Never use real secrets.
- A canary marked forbidden must occur zero times in durable outputs/logs/mock external payloads.
- `must_observe` means the scenario intentionally exposes that unit; `must_not_observe` means recording it as visible is a false positive.
- Timing expectations are ranges/tolerances, not invented exact precision.
- Later source re-fetch is a different acquisition and must never satisfy a historical `must_observe` expectation.
- Scenario changes that alter semantics should cite the product/design decision that changed the expected behavior.

## Initial scenarios

1. `ordinary-scroll` — viewport-only capture and scroll-away/redisplay behavior.
2. `privacy-redaction` — editable/password/iframe canaries must never leak while ordinary visible text remains observable.
3. `revisit` — leaving a page and returning must be a revisit, distinct from scrolling away and back.

Additional scenarios from `docs/EVALUATION.md` should be added incrementally as M1/M2 capabilities become executable.
