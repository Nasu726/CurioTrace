# CurioTrace Extension Bootstrap

Status: M1 implementation note for the browser-extension shell. Product permission/privacy semantics remain defined by `docs/PERMISSION_ONBOARDING.md`, `docs/NATIVE_HELPER_PROTOCOL.md`, and `docs/PRODUCT_SPEC.md`.

## 1. Manifest baseline

CurioTrace uses Manifest V3.

Required API permissions currently declared:

- `nativeMessaging`

Broad website access is **not** an install-time host permission. The only broad host patterns are declared under `optional_host_permissions`:

- `http://*/*`
- `https://*/*`

They are requested at runtime only through the explicit first-Start permission flow.

Do not add `scripting`, `tabs`, clipboard permissions, or other capabilities until the implementation that requires them is reviewed. M1 should not accumulate speculative install-time authority.

## 2. Cross-browser background entrypoint

Current browser behavior differs:

- Chromium MV3 uses `background.service_worker`;
- Firefox MV3 uses `background.scripts` and does not currently support extension `background.service_worker`.

The manifest therefore declares the same built module in both fields:

```text
background.scripts       = dist/background.js
background.service_worker = dist/background.js
background.type          = module
```

This follows the current MDN cross-browser Manifest V3 fallback guidance. Chrome uses the service worker entry; Firefox uses the scripts entry.

The implementation must remain valid in both a service-worker global and a Firefox non-persistent background-document context.

## 3. Native host identity

The implementation-level native host name is:

```text
uk.nasu.curiotrace
```

Firefox also needs a stable add-on ID so the native-host manifest can list the extension in `allowed_extensions`:

```text
curiotrace@nasu.uk
```

Chrome ignores `browser_specific_settings`; Chrome native-host authorization uses `allowed_origins` and the Chrome extension ID instead.

The Chrome extension ID/native-host manifest and Firefox native-host manifest are installer/build concerns and are not yet implemented here.

## 4. Background responsibilities

`apps/extension/src/background.ts` installs the production background shell.

It wires:

```text
WebExtension runtime
  -> BackgroundSessionBroker
  -> HelperConnectionController
  -> NativeMessagingTransport
  -> local helper
```

The background module does **not** start recording on load.

`BackgroundSessionBroker` currently accepts control requests only from an own-extension page:

- `sender.id` must equal the current extension ID;
- a tab/content-script sender is rejected;
- the sender URL must use an extension-page scheme (`chrome-extension://` or `moz-extension://`).

This prevents ordinary content scripts/page contexts from becoming a Start-control surface.

## 5. Double permission gate

The popup/onboarding permission controller is the primary UX gate and is responsible for displaying the explanation and invoking `permissions.request()` directly from the user's Continue action.

The background independently calls `permissions.contains()` before forwarding `session.start` to the helper.

Therefore:

```text
UI permission gate fails
  -> no background Start request

UI is bypassed or state is stale, but permission is absent
  -> background rejects HOST_PERMISSION_REQUIRED
  -> no native helper Start
```

The background recheck is not a substitute for the explanation/user-action UX. It is a second fail-closed implementation boundary.

## 6. Service-worker lifetime

Chrome documents that an active `runtime.connectNative()` connection keeps an extension service worker alive. CurioTrace may benefit from this while recording, but does not treat it as the only safety mechanism.

Unexpected extension/background termination still follows the existing failure model:

- native port disappears;
- extension-local authority is lost;
- helper process/connection lifetime ends as applicable;
- next helper connection performs a fresh handshake;
- durable helper authority converts unfinished `RECORDING`/`PAUSED` to `INTERRUPTED` with a fresh epoch before exposing state.

Do not add artificial heartbeat APIs merely to keep the extension alive continuously.

## 7. Firefox AMO disclosure is a release gate

Current Firefox signing/submission rules require `browser_specific_settings.gecko.id` for Manifest V3 signing, and new AMO submissions also require `data_collection_permissions` declarations.

The stable Gecko ID is fixed now because Native Messaging needs it.

The `data_collection_permissions` categories are **not** guessed in this implementation PR. CurioTrace handles browsing-derived information and sends some data from the extension to its local native helper, so the final AMO declaration must be reconciled with the public privacy policy, Store policy interpretation, native-helper boundary, and issue #22 before release.

## 8. Still pending

- actual popup/onboarding UI;
- popup-side direct `permissions.request()` event wiring;
- Pause/Resume/Stop UI wiring;
- browser event/content collectors;
- `scripting` permission, only when collector implementation needs it;
- native-host installer/manifest registration on each OS/browser;
- actual Chrome/Edge/Firefox unpacked/signed integration tests;
- final AMO data-collection declaration under #22.
