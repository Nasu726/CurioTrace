# CurioTrace

CurioTrace is a local-first browsing-session observation tool for turning explicitly recorded web research into reusable session context.

The project records what was actually visible and interacted with during an explicitly started session, reconstructs session-local exposure/revisits, and exports machine-readable data plus Markdown suitable for downstream LLM workflows.

CurioTrace deliberately stops at the single-session boundary. Long-term Wiki maintenance, cross-session synthesis, and persistent interest/profile modeling belong to downstream systems.

## Project status

The product semantics and privacy boundaries are being fixed before implementation architecture is chosen.

- Product specification: [`docs/PRODUCT_SPEC.md`](docs/PRODUCT_SPEC.md)
- Decision log: [`docs/DECISIONS.md`](docs/DECISIONS.md)
- Roadmap: #17
- Capture-architecture spike: #8

## Core principles

- Observation and inference are separate.
- Tracking starts only through explicit user action.
- Local-first by default.
- Privacy filtering happens as early as technically feasible.
- Raw screenshots are ephemeral by default.
- LLM assistance is replaceable and downstream from recording.
- Source observations remain reusable for later re-analysis.
