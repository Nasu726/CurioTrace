# M1 Navigation and Visibility Collector

Status: production implementation under M1 / issue #9, pending PR validation and real-browser verification under #8.

## Purpose

This collector establishes the minimum browser-side observation stream before DOM/content capture is enabled.

It records only:

- view/navigation continuity;
- active/background tab facts;
- browser-window focus facts without claiming occlusion visibility;
- privacy decisions for private, restricted, or excluded views;
- explicit gap/error observations when allowed browser metadata cannot be read.

It does **not** extract page body text, DOM semantic units, screenshots, OCR, form values, or clipboard contents.

## Capture-authority boundary

Browser listeners may remain registered while CurioTrace is idle, but each event first calls `CaptureAuthority.beginCapture()`.

If authority is absent (IDLE, PAUSED, FINISHED, INTERRUPTED, disconnected, or locally suspended):

- no observation is built;
- activation handling does not call `tabs.get()`;
- no source URL/title is evaluated or sent to the helper.

Async work revalidates the capture token before reading tab metadata and again when materializing the durable event. Pause/Stop/epoch change/disconnect therefore invalidates queued work.

## Start and Resume snapshots

A session cannot be reported as successfully started/resumed merely because the helper accepted the lifecycle transition.

After helper-authoritative Start/Resume:

1. the collector queries the currently active tab in the last-focused browser window;
2. it creates a fresh CurioTrace `view_id`;
3. it submits navigation/privacy continuity;
4. it submits the active-tab visibility fact.

Start clears prior-session in-memory view continuity. Resume intentionally creates a fresh view segment so the paused interval cannot be treated as continuous exposure.

If the collector is missing or the initial snapshot fails in a way that cannot be durably represented, the background broker disconnects the helper and reports Start/Resume failure instead of continuing a partial recording.

## Privacy precedence

Before a URL/title becomes a durable `source` pointer, `NavigationPrivacyGate` evaluates:

1. private/incognito state;
2. unknown private status (fail closed);
3. HTTP/HTTPS eligibility versus browser/internal/restricted schemes;
4. user tab/page/domain exclusions.

For denied views:

- the URL/title remains transient in extension memory only for gating;
- the durable navigation event contains continuity (`view_id`, transition kind) but no source pointer;
- a `privacy_decision` with `capture_mode=blocked` records only the denial reason and optional non-content rule ID.

Runtime browser tab/window IDs are never written into durable observation events.

## Visibility semantics

The collector records browser facts rather than inferred human visibility:

- tab activation/backgrounding is recorded as active-state evidence;
- losing browser-window focus is recorded with `exposure_visibility=unknown`;
- focus loss is **not** treated as proof that the page became invisible, because a non-focused window may remain visible side-by-side.

Later exposure reconstruction interprets these facts according to `docs/PRODUCT_SPEC.md`.

## Observation acknowledgements

`ProtocolObservationPort` consumes helper observation acknowledgements.

If the helper reports an authoritative state change such as durable-store failure -> `INTERRUPTED`, the extension applies that helper state immediately and invalidates existing capture tokens.

Transport/malformed-authority failures suspend local capture. Unexpected collector exceptions call the production fatal handler, which disconnects Native Messaging so no uncertain capture authority remains active.

## Current API baseline

The M1 baseline uses ordinary WebExtension APIs:

- `tabs.onActivated`;
- `tabs.onUpdated` for URL changes;
- `tabs.onRemoved`;
- `windows.onFocusChanged`;
- `tabs.get()` only after capture authority is confirmed;
- `tabs.query({active: true, lastFocusedWindow: true})` for Start/Resume snapshots.

The existing broad HTTP/HTTPS host permission supplies access to allowed URL/title metadata. This collector does not add an install-time `tabs` permission.

## Deliberately pending

Still outside this PR/collector boundary:

- SPA route detection beyond browser-observable tab URL changes;
- DOM/content semantic capture;
- screenshot/redaction/fingerprint capture;
- persistent user-exclusion storage/UI wiring;
- OS lock/suspend integration;
- exact browser-specific behavior verification in Chrome/Edge/Firefox;
- #8 architecture measurements and browser-store disclosure work in #22.
