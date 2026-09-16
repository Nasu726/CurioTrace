const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const SESSION_HOSTS = ["http://*/*", "https://*/*"];

async function requestSessionHostAccess() {
  try {
    const alreadyGranted = await chrome.permissions.contains({ origins: SESSION_HOSTS });
    if (alreadyGranted) return { granted: true, prompted: false, error: null };

    const granted = await chrome.permissions.request({ origins: SESSION_HOSTS });
    return { granted, prompted: true, error: null };
  } catch (error) {
    return {
      granted: false,
      prompted: false,
      error: String(error?.message || error)
    };
  }
}

async function injectContentScript(tabId) {
  try {
    await chrome.scripting.executeScript({
      target: { tabId },
      files: ["content.js"]
    });
    return { ok: true, error: null };
  } catch (error) {
    return { ok: false, error: String(error?.message || error) };
  }
}

chrome.action.onClicked.addListener(async (tab) => {
  const startedAt = performance.now();
  const permission = await requestSessionHostAccess();
  const injection = await injectContentScript(tab.id);

  let report = null;
  let domError = injection.error;
  let maskApplied = false;
  let screenshot = null;
  let captureError = null;

  if (injection.ok) {
    try {
      report = await chrome.tabs.sendMessage(tab.id, { type: "REPORT" });
      await chrome.tabs.sendMessage(tab.id, { type: "APPLY_MASKS" });
      maskApplied = true;
      await sleep(80);
    } catch (error) {
      domError = String(error?.message || error);
    }
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
      permission,
      injection,
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
