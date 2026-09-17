# GUI Review Gate

Status: required review process for every CurioTrace pull request that changes a user-visible screen, state, layout, wording, focus behavior, or interaction.

Automated tests can prove state-machine and DOM invariants, but they do not prove that a rendered interface is readable, usable, accessible, or visually intact. A GUI-changing PR therefore requires an actual rendered review before merge.

## Accessibility target

Use WCAG 2.2 Level AA as the minimum normative accessibility target where its success criteria apply. This is a review target, not a blanket conformance claim for an unfinished product.

Relevant baseline checks include:

- ordinary text contrast of at least 4.5:1 (large text may use the WCAG large-text threshold);
- controls and meaningful non-text visual states with sufficient contrast;
- keyboard-operable controls and visible focus;
- no focus hidden by author-created layout;
- pointer targets at least 24 by 24 CSS pixels or satisfying the WCAG spacing exception;
- state/error meaning not conveyed through color alone;
- meaningful status changes exposed to assistive technology.

Reference: W3C Web Content Accessibility Guidelines (WCAG) 2.2 and WAI guidance.

## Required rendered states

Review every state materially affected by the change. For the toolbar popup this normally includes:

- IDLE / Not recording;
- RECORDING;
- PAUSED;
- INTERRUPTED;
- FINISHED;
- helper unavailable/error;
- first-Start host-permission explanation;
- permission denied/revoked recovery;
- busy/in-progress state.

Do not infer one state from another when action sets, copy, or hierarchy differ.

## Required visual environments

At minimum inspect:

1. light appearance;
2. dark appearance;
3. normal popup width;
4. the narrowest supported width;
5. a large-text / approximately 200% zoom equivalent.

Check for:

- clipping or overlapping text;
- horizontal scrolling caused by authored layout;
- controls pushed out of view;
- labels truncated without a usable alternative;
- excessive whitespace or dense unreadable blocks;
- visual hierarchy that changes unpredictably between states.

## Keyboard and focus review

Exercise the complete primary flow without a pointer.

Verify:

- tab order follows the visual/task order;
- every interactive element has a clearly visible focus indicator;
- opening an explanation/recovery panel moves focus to an appropriate heading or primary action instead of leaving focus on a newly hidden control;
- closing/cancelling a secondary panel returns focus to a meaningful control;
- disabled controls do not trap focus;
- destructive and non-destructive actions remain distinguishable without relying only on color.

## Screen-reader/status semantics

Inspect the rendered DOM/accessibility semantics for:

- one clear page/panel heading for the current task;
- accessible button names that describe the action;
- `aria-live`/status output used deliberately rather than announcing every cosmetic change;
- busy state exposed with `aria-busy` or an equivalent status where waiting is user-visible;
- decorative status indicators hidden from assistive technology unless they carry independent meaning;
- technical reason codes kept secondary to human-readable recovery instructions.

## Information-design review

Accessibility conformance alone is not sufficient. Ask whether the screen supports CurioTrace's task with low cognitive load.

For each state, a user should be able to answer quickly:

1. Is CurioTrace recording right now?
2. What browsing information can be observed in this state?
3. What will the primary action do?
4. If something failed, what can I do next?
5. Is a permission request clearly distinguished from starting recording or external-AI disclosure?

Prefer short, scannable explanations over dense legalistic paragraphs. Put the decision-relevant consequence before implementation detail.

## Visual evidence

For GUI-changing PRs, produce screenshots or equivalent rendered captures for the affected representative states. Review them visually before merge; generating the files alone does not satisfy this gate.

Record in the PR/review notes:

- states/environments inspected;
- keyboard/focus result;
- contrast/readability result;
- any issue found and the commit that fixed it;
- any limitation that still requires real-browser or assistive-technology verification.

If the environment cannot render the affected GUI, mark the GUI review as pending and keep the PR unmerged until the required inspection can be completed elsewhere.
