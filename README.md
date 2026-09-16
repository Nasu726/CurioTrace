# CurioTrace

CurioTrace is a local-first browsing-session observation tool for turning explicitly recorded web research into reusable session context.

It records evidence of what was actually visible and interacted with during an explicitly started session, reconstructs session-local exposure/revisits, and produces compact machine-readable observations plus Markdown suitable for downstream LLM workflows such as an LLM Wiki.

CurioTrace deliberately stops at the single-session boundary. Long-term Wiki maintenance, cross-session synthesis, and persistent interest/profile modeling belong to downstream systems.

## Project status

Product/privacy/copyright semantics are substantially fixed. The leading capture architecture is a **DOM-first hybrid WebExtension + native helper**, but issue #8 remains open until the real-browser capture matrix is measured on normal Chrome/Edge environments and checked for Firefox parity.

Browser-independent M1 work is already in progress against fixed protocol/schema contracts.

- Product specification: [`docs/PRODUCT_SPEC.md`](docs/PRODUCT_SPEC.md)
- Decision log: [`docs/DECISIONS.md`](docs/DECISIONS.md)
- Capture architecture: [`docs/CAPTURE_ARCHITECTURE.md`](docs/CAPTURE_ARCHITECTURE.md)
- Native-helper protocol: [`docs/NATIVE_HELPER_PROTOCOL.md`](docs/NATIVE_HELPER_PROTOCOL.md)
- Observation schema: [`docs/OBSERVATION_SCHEMA.md`](docs/OBSERVATION_SCHEMA.md)
- Permission onboarding: [`docs/PERMISSION_ONBOARDING.md`](docs/PERMISSION_ONBOARDING.md)
- Evaluation contract: [`docs/EVALUATION.md`](docs/EVALUATION.md)
- M1 implementation plan: [`docs/M1_IMPLEMENTATION_PLAN.md`](docs/M1_IMPLEMENTATION_PLAN.md)
- Machine-readable observation schema: [`schemas/observation-v1.schema.json`](schemas/observation-v1.schema.json)
- Roadmap: #17
- Capture-architecture spike: #8
- Browser-store/privacy release gate: #22

## Current reference code

Reference/spike code is intentionally separated from future production-runtime choices:

- `spikes/capture-poc/` — disposable WebExtension harness for runtime permissions, DOM/redaction geometry, viewport capture, PDF/restricted-surface behavior and browser measurements.
- `spikes/native_helper_harness/` — zero-dependency Python Native Messaging/session-authority reference harness.
- `spikes/extension_authority_harness/` — pure JavaScript capture-authority and native-protocol client state machines.
- `tests/` — browser-independent privacy/protocol invariants and later golden scenarios.

GitHub Actions runs the native-helper and extension-authority suites as independent jobs. The protocol/privacy reference suite has already passed its initial CI run; browser-dependent #8 measurements remain intentionally separate.

## Core principles

- Observation and inference are separate.
- Tracking starts only through explicit user action.
- Broad host permission is requested only when first needed for Start, with an explanation first; refusal means recording does not start.
- Browser permission is capability, not always-on recording authorization.
- Local-first by default.
- Privacy filtering happens as early as technically feasible.
- Known sensitive/editable raster regions are redacted before viewport data crosses from extension to helper.
- Raw/unredacted screenshots and near-complete third-party page text are transient by default, not long-term archives.
- Helper disconnect, Pause, Stop and Interrupted states fail closed; stale asynchronous capture results are rejected by recording epoch/connection generation.
- High-quality semantic compression is delegated to replaceable external frontier-model agents through MCP; external AI transfer requires separate authorization.
- Finalized `session.md` is designed to become an immutable source artifact for a downstream LLM Wiki, not a finished cross-session knowledge base.
