# Platform storage paths

Status: M1 engineering contract. Product privacy/retention semantics remain governed by `docs/PRODUCT_SPEC.md` and `docs/DECISIONS.md`.

## Purpose

CurioTrace needs a predictable per-user local root for durable observation logs and helper-authoritative control state. This layer chooses and validates that filesystem location; it does **not** enable recording or provide encryption keys.

The baseline layout is:

```text
<CurioTrace root>/
  observations/   # AES-GCM encrypted per-session observation logs
  authority/      # helper session-authority control journal
```

Encryption keys are not files under this root. They belong to the OS credential store through the `SystemKeyProvider` adapter.

## Platform roots

### Windows

```text
%LOCALAPPDATA%\CurioTrace
```

CurioTrace deliberately uses `LOCALAPPDATA`, not roaming `APPDATA`, because browsing observations are local application data and should not be automatically moved between Windows profiles by the roaming-profile mechanism.

If `LOCALAPPDATA` is unavailable, helper bootstrap fails closed rather than silently selecting the working directory or another generic path.

### macOS

```text
~/Library/Application Support/CurioTrace
```

The user home must be resolvable. CurioTrace does not fall back to the working directory or `/tmp` for durable data.

### Linux

Use absolute `XDG_DATA_HOME` when configured:

```text
$XDG_DATA_HOME/curiotrace
```

Otherwise:

```text
~/.local/share/curiotrace
```

A relative `XDG_DATA_HOME` is ignored because the XDG Base Directory contract requires an absolute path. If no absolute XDG data root and no user home are available, bootstrap fails closed.

## Directory safety

Before the durable stores are opened, CurioTrace creates/validates the root plus `observations` and `authority` subdirectories.

Baseline checks:

- resolved root must be absolute and must not be the filesystem root;
- observations/authority must be direct, distinct children of the CurioTrace root;
- a managed path that already exists as a symlink or non-directory is rejected;
- managed directories are requested as `0700` on POSIX;
- observation/state files retain their own `0600`-style baseline where supported.

Rejecting the final managed directory symlink prevents a pre-existing link from silently redirecting CurioTrace's durable data. This is **not** a claim of complete local-filesystem anti-TOCTOU protection against a hostile process with the same user privileges. Parent-directory symlinks supplied by the user's normal home/platform configuration may still exist.

Windows filesystem confidentiality is governed primarily by the user's profile/ACL rather than POSIX mode bits; Go permission modes must not be presented as a Windows ACL guarantee.

## Backup/sync responsibility boundary

CurioTrace does not place durable data in browser sync storage or intentionally upload this root to a cloud service.

However, application-local files may still be copied by user-configured or OS-level backup, snapshot, migration, endpoint-management, or filesystem tools. CurioTrace does not claim that choosing these platform-local directories prevents such copies. The durable observation payload is therefore encrypted independently of directory placement.

The authority journal contains random session identity and lifecycle control metadata but no captured browsing content. Observation records remain under the authenticated-encryption boundary described in `docs/ENCRYPTED_STORAGE.md`.

## Bootstrap status

`apps/helper/internal/platformpath` implements the path policy and directory validation independently from the concrete OS credential-store adapter.

Resolving/creating these directories does **not** make the helper ready to record. Normal Start still requires:

1. a durable helper authority repository;
2. the AES-GCM observation store;
3. a production `SystemKeyProvider` backed by the required native OS secret store;
4. successful readiness checks for all required components.

No missing component may be replaced by a plaintext/testing fallback in ordinary recording.
