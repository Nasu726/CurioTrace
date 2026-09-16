const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

chrome.action.onClicked.addListener(async (tab) => {
  const startedAt = performance.now();
  let report = null;
  let domError = null;
  let maskApplied = false;
  let screenshot = null;
  let captureError = null;

  try {
    report = await chrome.tabs.sendMessage(tab.id, { type: "REPORT" });
    await chrome.tabs.sendMessage(tab.id, { type: "APPLY_MASKS" });
    maskApplied = true;
    await sleep(80);
  } catch (error) {
    domError = String(error?.message || error);
  }

  const captureStartedAt = performance.now();
  try {
    screenshot = await chrome.tabs.captureVisibleTab(tab.windowId, { format: "png" });
  } catch (error) {
    captureError = String(error?.message || error);
  }
  const captureMs = performance.now() - captureStartedAt;

  if (maskApplied) {
    try {
      await chrome.tabs.sendMessage(tab.id, { type: "CLEAR_MASKS" });
    } catch (_) {
      // Disposable PoC: navigation may have invalidated the content script.
    }
  }

  await chrome.storage.local.set({
    lastCapturePoc: {
      capturedAt: new Date().toISOString(),
      tab: { id: tab.id, url: tab.url, title: tab.title },
      report,
      domError,
      captureError,
      captureMs,
      totalMs: performance.now() - startedAt,
      screenshot
    }
  });

  await chrome.tabs.create({ url: chrome.runtime.getURL("result.html") });
});
