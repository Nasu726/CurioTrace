const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const SESSION_HOSTS = ["http://*/*", "https://*/*"];

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

async function runCapturePoc() {
  const permissionGranted = await chrome.permissions.contains({ origins: SESSION_HOSTS });
  if (!permissionGranted) {
    return {
      ok: false,
      message: "Required website-access permission is missing. No capture was performed."
    };
  }

  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (!tab?.id) {
    return { ok: false, message: "No active tab is available for the capture test." };
  }

  const startedAt = performance.now();
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
      permission: { granted: true },
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
  return { ok: true };
}

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message?.type !== "RUN_CAPTURE_POC") return;

  runCapturePoc()
    .then(sendResponse)
    .catch((error) => sendResponse({ ok: false, message: String(error?.message || error) }));

  return true;
});
