# CurioTrace CLI

Status: M1 CLI baseline following D013 / issue #45.

CurioTrace is CLI/helper-first. The human-facing CLI binary is `curiotrace`; the browser Native Messaging host remains a separate `curiotrace-helper` binary.

## Commands

### `curiotrace status`

Reports the current platform-local CurioTrace paths and whether production recording / encrypted session inspection are actually wired.

Use `--json` for machine-readable output.

The status command is intentionally honest about incomplete production bootstrap. Until the native OS secret-store adapter and encrypted-store bootstrap are connected, it reports recording/inspection as unavailable. It must not silently create a plaintext/test fallback merely to report readiness.

### `curiotrace inspect --session <session-id>`

Displays already-validated durable observations for one session.

Use `--json` to emit the validated event array without converting it into a separate presentation schema.

The command validates the production `ses_<32 lowercase hex>` identifier before any store lookup.

The inspection core is implemented, but the normal production binary does not yet have an encrypted `SessionReader` because the OS secret-store adapter/bootstrap remains pending. In that state the command exits explicitly with `SESSION_INSPECTION_UNAVAILABLE`.

This is deliberate fail-closed behavior. Do not wire the testing plaintext codec or a generic file-backed key fallback into normal CLI inspection.

## Exit codes

- `0`: command completed successfully;
- `1`: operational failure;
- `2`: invalid command/arguments;
- `3`: requested capability is intentionally unavailable in the current production wiring.

## Output contract

Human-readable output is intended for direct local inspection. JSON mode exists for scripts and downstream tooling.

CLI diagnostics must not dump rejected raw protocol payloads, unredacted raster data, secret-store key material, or browsing content merely because an operation failed.

## Relationship to the browser popup

The popup remains a minimal browser control plane for Start/Pause/Resume/Stop and runtime host-permission consent. General inspection, diagnostics, export, cleanup, and integration controls should grow here in the CLI rather than turning the popup into a management application.

## Pending work

- concrete Windows Credential Manager / macOS Keychain / Linux Secret Service adapter;
- production encrypted-store `SessionReader` wiring through a read-only path that never opens/mutates helper authority;
- session catalog/listing without requiring the caller to already know a session ID;
- helper/storage `doctor` diagnostics;
- export and deletion/cleanup commands;
- MCP/summarizer integration status.


## Authority safety boundary

The authoritative helper bootstrap in `apps/helper/internal/bootstrap` must not be reused by a separate CLI process for inspection. Opening helper authority has restart semantics and may convert durable `RECORDING` / `PAUSED` to `INTERRUPTED`.

The future production CLI reader must therefore open only the encrypted observation layer in a read-only/non-authoritative mode and must respect cross-process coordination with a live helper.

That read-only storage path is now implemented by `store.ReadOnlyFileStore` and `internal/inspection.Open`. It uses O_RDONLY log access, does not repair partial tails, does not call `CurrentKey`, and never opens helper authority. Default CLI wiring still awaits the native OS secret-store adapter.

See `docs/READ_ONLY_INSPECTION.md`.
