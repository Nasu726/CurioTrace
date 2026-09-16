const ext = globalThis.browser ?? chrome;
const SESSION_HOSTS = ["http://*/*", "https://*/*"];

async function injectContentScript(tabId) {
  try {
    await ext.scripting.executeScript({
      target: { tabId },
      files: ["content.js"]
    });
    return { ok: true, error: null };
  } catch (error) {
    return { ok: false, error: String(error?.message || error) };
  }
}

async function dataUrlToBitmap(dataUrl) {
  const response = await fetch(dataUrl);
  const blob = await response.blob();
  return createImageBitmap(blob);
}

async function blobToDataUrl(blob) {
  const bytes = new Uint8Array(await blob.arrayBuffer());
  let binary = "";
  const chunkSize = 0x8000;
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize));
  }
  return `data:${blob.type};base64,${btoa(binary)}`;
}

async function redactScreenshot(rawDataUrl, report) {
  const bitmap = await dataUrlToBitmap(rawDataUrl);
  const canvas = new OffscreenCanvas(bitmap.width, bitmap.height);
  const ctx = canvas.getContext("2d");
  ctx.drawImage(bitmap, 0, 0);

  const viewportWidth = report?.viewport?.width || bitmap.width;
  const viewportHeight = report?.viewport?.height || bitmap.height;
  const scaleX = bitmap.width / viewportWidth;
  const scaleY = bitmap.height / viewportHeight;
  const rects = Array.isArray(report?.sensitiveRects) ? report.sensitiveRects : [];

  ctx.fillStyle = "#ff00ff";
  for (const rect of rects) {
    ctx.fillRect(
      Math.max(0, rect.x * scaleX),
      Math.max(0, rect.y * scaleY),
      Math.max(0, rect.width * scaleX),
      Math.max(0, rect.height * scaleY)
    );
  }

  const redactedBlob = await canvas.convertToBlob({ type: "image/png" });
  return {
    dataUrl: await blobToDataUrl(redactedBlob),
    width: bitmap.width,
    height: bitmap.height,
    maskCount: rects.length
  };
}

async function inspectAndDiscardScreenshot(rawDataUrl) {
  const bitmap = await dataUrlToBitmap(rawDataUrl);
  return {
    width: bitmap.width,
    height: bitmap.height,
    discardedUnredacted: true
  };
}

async function runCapturePoc() {
  const permissionGranted = await ext.permissions.contains({ origins: SESSION_HOSTS });
  if (!permissionGranted) {
    return {
      ok: false,
      message: "Required website-access permission is missing. No capture was performed."
    };
  }

  const [tab] = await ext.tabs.query({ active: true, currentWindow: true });
  if (!tab?.id) {
    return { ok: false, message: "No active tab is available for the capture test." };
  }

  const startedAt = performance.now();
  const injection = await injectContentScript(tab.id);

  let report = null;
  let domError = injection.error;
  let screenshot = null;
  let screenshotInfo = null;
  let captureError = null;

  if (injection.ok) {
    try {
      report = await ext.tabs.sendMessage(tab.id, { type: "REPORT" });
    } catch (error) {
      domError = String(error?.message || error);
    }
  }

  const captureStartedAt = performance.now();
  try {
    const rawScreenshot = await ext.tabs.captureVisibleTab(tab.windowId, { format: "png" });

    if (report) {
      const redacted = await redactScreenshot(rawScreenshot, report);
      screenshot = redacted.dataUrl;
      screenshotInfo = {
        width: redacted.width,
        height: redacted.height,
        redactedBeforePersistence: true,
        maskCount: redacted.maskCount
      };
    } else {
      // Restricted/DOM-unavailable surfaces are fingerprint-only candidates.
      // The PoC proves capture success/dimensions without persisting the unredacted pixels.
      screenshotInfo = await inspectAndDiscardScreenshot(rawScreenshot);
    }
  } catch (error) {
    captureError = String(error?.message || error);
  }
  const captureMs = performance.now() - captureStartedAt;

  await ext.storage.local.set({
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
      screenshotInfo,
      screenshot
    }
  });

  await ext.tabs.create({ url: ext.runtime.getURL("result.html") });
  return { ok: true };
}

ext.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message?.type !== "RUN_CAPTURE_POC") return;

  runCapturePoc()
    .then(sendResponse)
    .catch((error) => sendResponse({ ok: false, message: String(error?.message || error) }));

  return true;
});
