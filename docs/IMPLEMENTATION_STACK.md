# CurioTrace Implementation Stack

Status: replaceable engineering baseline for M1. This is **not** a product-semantic contract; `docs/PRODUCT_SPEC.md` and `docs/DECISIONS.md` remain authoritative for user-visible behavior and safety invariants.

## 1. Repository shape

CurioTrace is intentionally a small monorepo because the browser extension, local native helper, shared schemas, tests, and documentation implement one tightly versioned product/protocol.

Planned top-level layout:

```text
apps/
  extension/        TypeScript WebExtension
  helper/           Go native helper
schemas/            transport-neutral durable schemas
docs/               product/design/implementation contracts
spikes/              disposable/reference harnesses
tests/               cross-component and golden tests
```

The existing `spikes/` implementations remain executable reference oracles while equivalent production modules are introduced under `apps/`.

## 2. Browser extension: TypeScript, minimal tooling

Baseline:

- TypeScript for type-checked protocol/capture-authority code;
- Manifest V3 for initial Chrome/Edge M1;
- no UI framework in M1;
- no architecture hidden behind a browser-extension framework until #8 empirical tests establish its value;
- browser APIs stay behind narrow adapters so Firefox parity does not require rewriting semantic logic.

Why:

- most extension behavior is event/protocol/state-machine code rather than complex UI;
- TypeScript can share the same structural concepts as JSON schemas without forcing a runtime dependency;
- keeping the initial bundle small makes permission/privacy behavior easier to audit;
- the already-tested Node reference state machine can be promoted with minimal semantic translation.

A bundler may be introduced if required by browser compatibility or dependency packaging, but higher layers must not depend on bundler/framework-specific semantics.

## 3. Native helper: Go

Baseline:

- Go native executable;
- standard-library Native Messaging framing/session authority first;
- application-private durable store behind an interface;
- OS-specific install/OCR/key-management adapters isolated from protocol/session semantics;
- MCP added later through the official MCP Go SDK once M5/M6 work begins.

Why Go for the initial production helper:

- straightforward small native executable with no separate language runtime required on the user machine;
- simple stdio/binary framing and concurrency model for Native Messaging;
- practical Windows/macOS/Linux build/distribution path;
- low idle overhead for a resident local helper;
- official Model Context Protocol Go SDK exists, reducing the need for a second helper runtime for the later MCP server;
- simpler implementation/packaging burden than adopting Rust solely for memory-safety/performance benefits that are not currently the bottleneck.

This decision does **not** claim that Go memory can be perfectly zeroized after sensitive processing. The product threat model does not promise defense against a fully compromised process/OS. Data minimization and short retention remain the primary controls.

## 4. Why not keep Python as the production helper

The Python helper under `spikes/native_helper_harness/` is a reference oracle, not a product commitment.

Python remains useful for tests and schema tooling, but shipping it as the normal desktop native host would add interpreter/environment/packaging complexity or require a bundled runtime. The production helper should be a self-contained application component.

The Python reference remains valuable as an independent implementation for cross-language conformance tests.

## 5. Why Rust is not the baseline

Rust is a credible alternative and has an official MCP SDK. It offers stronger low-level memory-safety/control and a good cross-platform native story.

It is not selected for M1 because CurioTrace's current risks are protocol correctness, privacy boundaries, browser behavior, capture fidelity, and packaging—not CPU throughput or unsafe native memory manipulation. Go gives a shorter implementation path while retaining a replaceable helper boundary.

Reconsider Rust if later evidence shows that image/OCR pipelines, zero-copy sensitive buffers, platform APIs, or memory-control requirements materially benefit from it.

## 6. Shared contract strategy

Do not directly share implementation source between TypeScript and Go. Share semantics through versioned machine-readable schemas and conformance fixtures:

- `schemas/observation-v1.schema.json`;
- Native Messaging protocol/version contract in `docs/NATIVE_HELPER_PROTOCOL.md`;
- golden/rejection fixtures;
- cross-language integration tests.

This prevents one runtime from silently becoming the definition of the protocol.

## 7. CI strategy

Independent work runs in parallel where possible:

- TypeScript/Node extension checks/tests;
- Go helper build/tests;
- Python independent reference/schema/privacy tests;
- cross-language protocol integration;
- golden-manifest validation.

Browser integration jobs are added separately once #8 can run in a suitable environment.

## 8. Deferred implementation choices

Still deliberately open:

- TypeScript bundler/build packager;
- concrete Go durable database/encryption library;
- native-host installer/updater;
- platform OCR adapters;
- browser compatibility abstraction/polyfill;
- production MCP tool/resource surface;
- UI visual framework.

Choose these when a concrete M1/M5 requirement forces the choice, not earlier.