# Native helper reference harness

This directory is a disposable, zero-dependency reference implementation for issues #23–#27. It is **not** a production language/runtime commitment.

It exists to test CurioTrace's semantic guarantees before the real browser/native stack is implemented:

- Native Messaging JSON framing;
- helper-authoritative session state;
- `recording_epoch` invalidation on Pause/Resume/Stop/Interrupted;
- fail-closed stale observation rejection;
- observation/privacy validation;
- non-echoing rejection acknowledgements.

## Run tests

From the repository root:

```bash
python -m unittest discover -s tests -p 'test_*.py' -v
```

## Run the reference host manually

From the repository root:

```bash
python -m spikes.native_helper_harness.server
```

It expects browser Native Messaging framing on stdin and emits only framed JSON on stdout. It intentionally persists nothing and does not log captured payloads.

## Important limitations

- No browser integration yet.
- No production database/encryption.
- No OCR/image transport implementation.
- No MCP server.
- No installer/native-host manifest.
- State is in memory only.
- It demonstrates contract semantics, not production robustness/performance.

The normative design documents are:

- `docs/NATIVE_HELPER_PROTOCOL.md`
- `docs/OBSERVATION_SCHEMA.md`
- `schemas/observation-v1.schema.json`
