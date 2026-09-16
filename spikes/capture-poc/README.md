# CurioTrace capture PoC

Disposable WebExtension harness for issue #8. It is intentionally not product code.

## What it tests

On extension-action click:

1. asks the top-frame content script for visible DOM text and sensitive/editable geometry;
2. overlays magenta masks on editable controls and iframe rectangles;
3. calls `tabs.captureVisibleTab()`;
4. removes the masks;
5. opens an extension result page showing the screenshot, DOM report, capture errors, and timing.

The harness deliberately keeps DOM observation and screenshot success/failure separate so restricted surfaces can be compared correctly.

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

Then click the extension action.

## Expected ordinary-page observations

The result should make these distinctions visible:

- ordinary visible paragraph: present in DOM report and screenshot;
- input/password/contenteditable regions: magenta-masked before screenshot;
- iframe rectangles: conservatively magenta-masked by this top-frame PoC;
- `API_KEY_EXAMPLE_123`: still visible, demonstrating that arbitrary confidential-looking page text cannot be reliably classified as secret;
- canvas text: visible in screenshot but absent from ordinary DOM text extraction;
- form values should not appear in the DOM text report.

This harness masks all iframes on purpose. Later variants can test per-frame injection and selective masking.

## PDF / restricted-surface test

Open a non-sensitive PDF in the browser's built-in PDF viewer and click the extension action.

Record separately:

- whether the content script responds (`domError`);
- whether `captureVisibleTab()` succeeds (`captureError` / screenshot).

Repeat on a harmless browser-internal/restricted page if the browser permits the action. Do not use a page containing sensitive account data.

The important property is that screenshot eligibility must not be inferred from DOM-injection eligibility.

## Navigation / permissions test

With the PoC loaded:

1. capture one origin;
2. navigate the same tab to another origin;
3. open another tab/origin;
4. capture again;
5. record permission prompts and whether DOM/screenshot behavior changes.

This informs whether CurioTrace requires broad host permissions for zero-friction session tracking.

## Scroll / rate test

Capture at the top of the fixture, scroll to the bottom paragraph, and capture again. Record `captureMs` and total latency.

The product should eventually schedule captures from navigation/scroll/resize/mutation signals with debounce/change detection rather than poll at a video-like rate. Chrome documents `captureVisibleTab()` as expensive and caps it at two calls per second.

## Result matrix

Fill this into issue #8 for each browser/surface:

| Browser/surface | DOM works | sensitive geometry known | screenshot works | pre-mask works | capture ms | notes |
|---|---:|---:|---:|---:|---:|---|
| Chrome ordinary HTML | | | | | | |
| Edge ordinary HTML | | | | | | |
| Chrome PDF viewer | | | | | | |
| Edge PDF viewer | | | | | | |
| cross-origin iframe | | | | | | |
| canvas/image text | | | | | | |
| restricted browser page | | | | | | |
| Firefox parity check | | | | | | |

## References

- Chrome tabs API: https://developer.chrome.com/docs/extensions/reference/api/tabs
- Chrome scripting API: https://developer.chrome.com/docs/extensions/reference/api/scripting
- MDN `tabs.captureVisibleTab`: https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/API/tabs/captureVisibleTab
- MDN content scripts: https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Content_scripts

## Safety

The PoC stores the last screenshot in extension local storage only so it can display the result page. This is **not** CurioTrace's intended screenshot-retention policy. Do not use the PoC on real sensitive pages. Remove the unpacked extension/profile after testing if desired.
