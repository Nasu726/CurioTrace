# CurioTrace M1 Popup Controls

Status: implementation note for the minimal M1 browser control plane. CurioTrace is CLI/helper-first; this popup exists only for browser-local lifecycle/permission duties.

## Purpose

The popup exposes explicit lifecycle control and current helper-authoritative session state. Opening the popup is not recording authorization.

Lifecycle actions shown by state:

| State | Actions |
| --- | --- |
| `IDLE` | Start |
| `RECORDING` | Pause, Stop |
| `PAUSED` | Resume, Stop |
| `INTERRUPTED` | Resume, Stop |
| `FINISHED` | Start |

The popup does not infer user intent from opening, focus, or repeated use.

## First Start permission flow

When Start is pressed:

1. check current HTTP/HTTPS host permission;
2. if absent, show the canonical CurioTrace explanation;
3. only an explicit Continue click calls `permissions.request()`;
4. if permission is denied/cancelled/fails, no helper Start occurs;
5. after permission is present, background rechecks it and establishes helper authority before Start;
6. UI reports `RECORDING` only from helper-authoritative state.

The Continue handler calls the permission-controller method directly from the click handler so the browser permission request can remain attached to the user gesture.

A real browser may close an extension popup while displaying its permission UI. If that occurs, the safe fallback is that permission may be granted but Recording is not assumed. Reopening the popup and pressing Start again sees the existing permission and proceeds without another broad permission prompt. This behavior must be measured in #8/M1 browser integration tests.

## State refresh

Opening/retrying the popup asks the background for state. The background establishes a fresh helper connection/handshake when needed before returning state.

This matters after background/helper restart: the popup must not display a default local `IDLE` state when durable helper authority actually requires `INTERRUPTED` recovery.

## Control failures

Pause/Resume/Stop use helper-authoritative control messages and epoch changes.

If transport becomes uncertain:

- local capture is suspended;
- the requested state change is not shown as successful;
- the popup refreshes state through the recovery path;
- no optimistic continuation of Recording is allowed.

Reason/error codes shown by the M1 popup are implementation diagnostics. They must not include captured page content.

## Deliberately minimal browser surface

This popup is not the primary CurioTrace application UI. It exists because lifecycle control should be reachable from the browsing context and runtime host-permission requests need a browser user gesture.

Keep here only:

- current recording/interrupted/helper state;
- Start / Pause / Resume / Stop;
- the permission explanation and browser permission request;
- small recovery guidance needed to avoid unsafe or confusing control flow.

Prefer the CLI for session inspection, diagnostics, export, deletion/cleanup, helper/storage status, MCP/integration status, and advanced controls.

Deferred/optional:

- polished branding/icons;
- rich Session Viewer;
- broad settings/management UI;
- diagnostics/repair wizard beyond concise browser guidance;
- localization and richer visual design.

The remaining popup still needs baseline keyboard, focus, readable contrast, and assistive-technology semantics. Accessibility is a property of the minimal control plane, not a reason to expand it.
