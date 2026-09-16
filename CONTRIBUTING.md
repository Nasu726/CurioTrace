# Contributing to CurioTrace

## Repository workflow

`main` is the integration branch and is treated as the repository source of truth.

All changes must follow this workflow:

1. create a dedicated branch from the current `main`;
2. make commits on that branch only;
3. run the relevant CI/tests;
4. open a pull request to `main`;
5. inspect the diff and CI result;
6. merge the pull request only after the checks relevant to the change are green or the failure is explicitly understood and documented.

Do **not** commit implementation or documentation changes directly to `main` during normal development.

The maintainer may merge PRs without an additional confirmation round when the change matches an already-approved direction and verification is green. Product decisions that require user judgment must still be raised before merging behavior that would lock in that decision.

## Branch naming

Use short purpose-oriented names, for example:

- `feat/m1-observation-pipeline`
- `fix/native-message-framing`
- `docs/privacy-contract`
- `spike/capture-chromium`

## Scope discipline

Keep each PR focused enough that its correctness and privacy impact can be reviewed independently. Do not mix unrelated cleanup into privacy-sensitive or protocol changes unless required for the change.

## Verification

CurioTrace treats privacy/capture-authority failures as correctness failures. Relevant changes should preserve or extend tests for:

- fail-closed session/capture authority;
- stale epoch/connection rejection;
- privacy-invalid observation rejection;
- forbidden content not reaching durable storage/logs;
- restart/interruption semantics;
- cross-language protocol compatibility where applicable.

Run independent test groups in parallel in CI where practical.

## Source of truth

Stable product decisions belong in `docs/DECISIONS.md` and `docs/PRODUCT_SPEC.md`. Replaceable implementation choices belong in implementation-specific documentation such as `docs/IMPLEMENTATION_STACK.md`.

When chat discussion changes an accepted design or implementation rule, update the repository in the same workstream so the repository does not lag behind the discussion.