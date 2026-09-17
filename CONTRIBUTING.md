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

### GUI-changing pull requests

Automated tests are necessary but **not sufficient** for any change that can alter a user-visible screen, state, layout, copy, focus behavior, or interaction.

Before merging a GUI-changing PR:

1. render the affected UI rather than reviewing markup/CSS alone;
2. inspect representative states in both light and dark appearance where supported;
3. check the narrowest supported viewport and a large-text / roughly 200% zoom condition for clipping, overflow, hidden controls, and unnecessary horizontal scrolling;
4. exercise the primary flow with keyboard only and verify logical focus order, visible focus, and sensible focus movement after panels/dialog-like views appear;
5. verify that state and errors are not conveyed by color alone and that text/control contrast remains readable;
6. check pointer target size/spacing, disabled/busy feedback, accessible names/status announcements, and destructive-action clarity;
7. review information hierarchy and cognitive load: the user should be able to identify current state, consequence, and next safe action quickly;
8. visually inspect screenshots/renders and record the result in the PR or review notes, including any known limitation.

Use `docs/GUI_REVIEW.md` as the detailed checklist. If actual rendering is unavailable in the current environment, the GUI-changing PR remains unverified and should not be merged merely because unit tests pass.

## Source of truth

Stable product decisions belong in `docs/DECISIONS.md` and `docs/PRODUCT_SPEC.md`. Replaceable implementation choices belong in implementation-specific documentation such as `docs/IMPLEMENTATION_STACK.md`.

When chat discussion changes an accepted design or implementation rule, update the repository in the same workstream so the repository does not lag behind the discussion.
