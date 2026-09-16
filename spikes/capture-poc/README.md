# CurioTrace capture PoC

Disposable WebExtension harness for issue #8. It is intentionally not product code.

## What it tests

On extension-action click:

1. shows a CurioTrace explanation of why cross-site website access is needed;
2. only after the user explicitly continues, requests broad HTTP/HTTPS host permission if it has not already been granted;
3. if the permission is denied, stops immediately without DOM observation or screenshot capture;
4. dynamically injects the top-frame probe where the browser allows it;
5. asks the probe for visible DOM text and sensitive/editable geometry;
6. calls `tabs.captureVisibleTab()`;
7. if DOM-safe geometry exists, redacts the raw screenshot **inside the extension process** using `OffscreenCanvas` before persistence;
8. if DOM-safe geometry is unavailable, records capture success/dimensions only and discards the unredacted screenshot without persistence;
9. opens an extension result page showing permission/injection state, redaction state, DOM report, capture errors, and timing.

The harness deliberately keeps host permission, DOM injection, screenshot acquisition, and redaction success/failure separate so restricted surfaces can be compared correctly.

## Why runtime host permission is part of the spike

The extension manifest uses `optional_host_permissions` instead of requiring `<all_urls>` at install time. Chrome and Firefox MV3 can request optional host access at runtime.

This lets the spike test the intended product distinction:

- installing CurioTrace does not itself authorize observation of every site;
- the first explicit recording action first explains why broad host access is necessary for a complete cross-site session;
- only after that explanation does CurioTrace trigger the browser's permission request;
- once granted, CurioTrace's own `IDLE` / `RECORDING` / `PAUSED` state remains the stricter capture authorization boundary;
- if the required broad host request is denied, CurioTrace does **not** start a partial recording.

The explanation should make clear that broad host capability is required because a recorded session may navigate across unrelated sites and tabs. It should also make clear what the permission does **not** mean: installation alone does not start capture, excluded/private pages remain outside the intended capture scope, and website-access permission is not consent to transmit browsing contents to an external model.

## Load

Chrome / Chromium / Edge:

1. Open the extensions management page.
2. Enable Developer mode.
3. Choose **Load unpacked** and select `spikes/capture-poc/extension`.
4. Pin the PoC action if useful.

Use a normal developer browser profile. Some managed browsers disable unpacked extensions.

## Serve fixtures

From the repository root, run two servers so the page includes both same-origin and cross-origin frames:

```bash
python -m http.server 8765 --directory spikes/capture-poc/fixtures
python -m http.server 8766 --directory spikes/capture-poc/fixtures
```

Open:

`http://127.0.0.1:8765/index.html`

Then click the extension action. On the first run, verify the sequence:

1. CurioTrace explanation;
2. explicit Continue action;
3. browser host-permission prompt;
4. capture only if permission is granted.

Also test denial: after rejecting the browser permission prompt, verify that no capture result is produced and the popup reports that recording did not start.

## Expected ordinary-page observations

The result should make these distinctions visible:

- ordinary visible paragraph: present in DOM report and persisted redacted screenshot;
- input/password/contenteditable rectangles: magenta-redacted **in the captured image**, without flashing overlays into the live page;
- iframe rectangles: conservatively magenta-redacted by this top-frame PoC;
- `API_KEY_EXAMPLE_123`: still visible, demonstrating that arbitrary confidential-looking page text cannot be reliably classified as secret;
- canvas text: visible in screenshot but absent from ordinary DOM text extraction;
- form values should not appear in the DOM text report.

This harness redacts all iframe rectangles on purpose. Later variants can test per-frame injection and selective masking.

### In-extension redaction boundary

The current safer candidate is:

1. collect trusted mask rectangles from the page;
2. capture the active-tab viewport transiently;
3. decode and redact the raster immediately inside the extension runtime using standard Canvas/`OffscreenCanvas` APIs;
4. only then persist or send the redacted representation to the native helper;
5. discard the unredacted raster.

This narrows the trusted data path compared with sending raw pixels to the native helper for masking. The native helper may still perform OCR, storage, matching, and MCP-related work on the already-redacted image.

The PoC stores a redacted screenshot in extension local storage only so the result page can display it. That is still PoC behavior, not the intended production retention policy.

## PDF / restricted-surface test

Open a non-sensitive PDF in the browser's built-in PDF viewer and click the extension action.

Record separately:

- whether dynamic content-script injection succeeds (`injection` / `domError`);
- whether `captureVisibleTab()` succeeds (`captureError` / `screenshotInfo`).

If DOM-safe geometry is unavailable, the PoC intentionally does **not** persist the screenshot. It records dimensions/capture success and discards the unredacted pixels. This models the current Tier-2 fingerprint-only direction.

Repeat on a harmless browser-internal/restricted page if the browser permits the action. Do not use a page containing sensitive account data.

The important property is that screenshot eligibility must not be inferred from DOM-injection eligibility. MDN documents that content scripts cannot run in the built-in PDF viewer and other privileged browser UI, while `captureVisibleTab()` can capture some otherwise restricted surfaces.

## Navigation / permissions test

With the PoC loaded:

1. note the first-click explanatory popup and host-permission prompt;
2. grant access and capture one origin;
3. navigate the same tab to another origin;
4. open another tab/origin;
5. capture again;
6. record whether the permission persists and whether DOM/screenshot behavior changes;
7. revoke the host permission in the browser's extension settings and repeat;
8. explicitly deny the next request and verify that capture does not begin.

This tests whether a one-time explained permission grant can support zero-friction recording while CurioTrace's own Start/Pause/Stop state controls actual capture.

## Scroll / rate test

Capture at the top of the fixture, scroll to the bottom paragraph, and capture again. Record `captureMs` and total latency.

The product should eventually schedule captures from navigation/scroll/resize/mutation signals with debounce/change detection rather than poll at a video-like rate. Chrome documents `captureVisibleTab()` as expensive and caps it at two calls per second.

## Result matrix

Fill this into issue #8 for each browser/surface:

| Browser/surface | host permission | DOM works | sensitive geometry known | screenshot works | in-extension redaction works | capture ms | notes |
|---|---|---:|---:|---:|---:|---:|---|
| Chrome ordinary HTML | | | | | | | |
| Edge ordinary HTML | | | | | | | |
| Chrome PDF viewer | n/a/restricted | | | | n/a/fingerprint-only | | |
| Edge PDF viewer | n/a/restricted | | | | n/a/fingerprint-only | | |
| cross-origin iframe | | | | | | | |
| canvas/image text | | | | | | | |
| restricted browser page | n/a/restricted | | | | should be denied | | |
| Firefox parity check | | | | | | | |

## References

- Chrome tabs API: https://developer.chrome.com/docs/extensions/reference/api/tabs
- Chrome scripting API: https://developer.chrome.com/docs/extensions/reference/api/scripting
- Chrome permissions API: https://developer.chrome.com/docs/extensions/reference/api/permissions
- Chrome native messaging: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging
- MDN `tabs.captureVisibleTab`: https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/API/tabs/captureVisibleTab
- MDN content scripts: https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Content_scripts
- MDN optional host permissions: https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/manifest.json/optional_host_permissions
- MDN `OffscreenCanvas`: https://developer.mozilla.org/en-US/docs/Web/API/OffscreenCanvas
- MDN native messaging: https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Native_messaging

## Safety

Do not use the PoC on real sensitive pages. On DOM-inspectable pages, only the redacted screenshot is persisted by the PoC. On DOM-unavailable surfaces, screenshot pixels are discarded without persistence after capture metadata is obtained. Remove the unpacked extension/profile after testing if desired.
