# Native helper reference harness

This directory is a disposable, zero-dependency reference implementation for issues #23–#27 and M1 lifecycle testing. It is **not** a production language/runtime or database commitment.

It exists to test CurioTrace's semantic guarantees before the real browser/native stack is implemented:

- Native Messaging JSON framing;
- helper-authoritative session state;
- `recording_epoch` invalidation on Pause/Resume/Stop/Interrupted;
- fail-closed stale observation rejection;
- observation/privacy validation;
- non-echoing rejection acknowledgements;
- restart recovery that converts unfinished sessions to `INTERRUPTED` with a fresh epoch;
- rejection of privacy-invalid observations before reference persistence.

## Run tests

From the repository root:

```bash
python -m unittest discover -s tests -p 'test_*.py' -v
```

## Run the in-memory reference host manually

From the repository root:

```bash
python -m spikes.native_helper_harness.server
```

It expects browser Native Messaging framing on stdin and emits only framed JSON on stdout. It intentionally persists nothing and does not log captured payloads.

## Persistence reference

`persistent.py` contains a small SQLite-backed wrapper used only to verify lifecycle/persistence semantics:

- validated compact observations survive store reopen;
- a stored `RECORDING` or `PAUSED` session is never silently restored after helper process restart;
- restart converts the unfinished session to `INTERRUPTED` and advances `recording_epoch`;
- old-epoch observations remain rejected;
- a clean `FINISHED` session is not resurrected;
- privacy-invalid events are rejected before they enter the reference store.

The SQLite file is **not** the production storage/security design. It deliberately does not claim production encryption-at-rest, platform key-store integration, migrations, backup behavior, or hardened concurrency.

## Important limitations

- No real browser integration yet.
- No production database/encryption implementation.
- No OCR/image transport implementation.
- No MCP server.
- No installer/native-host manifest.
- The executable reference server remains in-memory; persistence is exercised through the test wrapper.
- It demonstrates contract semantics, not production robustness/performance.

The normative design documents are:

- `docs/NATIVE_HELPER_PROTOCOL.md`
- `docs/OBSERVATION_SCHEMA.md`
- `docs/M1_IMPLEMENTATION_PLAN.md`
- `schemas/observation-v1.schema.json`
