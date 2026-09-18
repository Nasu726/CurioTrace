# CurioTrace Control Surfaces

Status: product/implementation guidance following decision D013 / issue #45.

## Principle

CurioTrace is CLI/helper-first.

The product's core value is reliable session capture, privacy/copyright boundaries, local persistence, reconstruction, and downstream handoff. User-interface surface area should remain proportional to tasks that genuinely require it.

## Minimal browser control plane

The browser extension popup is retained only because some actions are inherently browser-local:

- explicit Start / Pause / Resume / Stop near the browsing context;
- explanation of broad HTTP/HTTPS access before first use;
- runtime browser permission request from a direct user gesture;
- minimal recording / paused / interrupted / helper-unavailable state;
- concise recovery guidance when a browser-local action cannot proceed.

Opening the popup never authorizes recording by itself.

Do not turn the popup into a general management application.

## CLI-first responsibilities

Prefer CLI/text commands for:

- list/inspect sessions;
- show one session's chronology, sources, gaps, capture modes, and metadata;
- export machine-readable session data or finalized Markdown;
- inspect helper/storage/key/backend health;
- cleanup/delete sessions and temporary material;
- diagnose installation/native-host problems;
- inspect MCP/summarizer integration state;
- developer/advanced controls.

Commands should support both human-readable and machine-readable output where practical.

Current M1 baseline:

- `curiotrace status [--json]` reports platform paths and truthful production readiness;
- `curiotrace inspect --session <id> [--json]` defines single-session validated-event inspection;
- production encrypted inspection remains unavailable until the native secret-store/bootstrap path is wired, and the CLI fails explicitly rather than falling back to plaintext.

See `docs/CLI.md`.

## Optional GUI

A richer GUI is allowed later, but it is not a milestone by default. Add one only when a concrete workflow is materially worse in CLI/text form.

Examples that might justify a later GUI include a genuinely useful visual session reconstruction or exclusion-management task. Such work should be justified separately rather than inherited as a standing roadmap obligation.

## Accessibility

The minimal browser control plane must remain keyboard-operable, readable, and semantically usable with assistive technology. Accessibility review should focus on the small surface that actually remains; it should not expand product scope merely to justify GUI infrastructure.
