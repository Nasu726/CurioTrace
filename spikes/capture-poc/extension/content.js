(() => {
  if (globalThis.__curioTraceCapturePocInstalled) return;
  globalThis.__curioTraceCapturePocInstalled = true;

  const MASK_ATTR = "data-curiotrace-poc-mask";

  function isEditable(el) {
    return Boolean(
      el.closest("input, textarea, select, [contenteditable=''], [contenteditable='true']")
    );
  }

  function visibleText() {
    const walker = document.createTreeWalker(document.body || document.documentElement, NodeFilter.SHOW_TEXT);
    const parts = [];
    while (walker.nextNode()) {
      const node = walker.currentNode;
      const parent = node.parentElement;
      if (!parent || isEditable(parent)) continue;
      const text = node.textContent?.replace(/\s+/g, " ").trim();
      if (!text) continue;
      const rect = parent.getBoundingClientRect();
      if (rect.bottom <= 0 || rect.top >= innerHeight || rect.right <= 0 || rect.left >= innerWidth) continue;
      parts.push(text);
      if (parts.join(" ").length > 2000) break;
    }
    return parts.join(" ").slice(0, 2000);
  }

  function rectOf(el) {
    const r = el.getBoundingClientRect();
    return {
      x: Math.max(0, r.left),
      y: Math.max(0, r.top),
      width: Math.max(0, Math.min(innerWidth, r.right) - Math.max(0, r.left)),
      height: Math.max(0, Math.min(innerHeight, r.bottom) - Math.max(0, r.top)),
      tag: el.tagName.toLowerCase(),
      type: el instanceof HTMLInputElement ? el.type : undefined
    };
  }

  function sensitiveRects() {
    return [...document.querySelectorAll("input, textarea, select, [contenteditable=''], [contenteditable='true'], iframe")]
      .map(rectOf)
      .filter((r) => r.width > 0 && r.height > 0);
  }

  function clearMasks() {
    document.querySelectorAll(`[${MASK_ATTR}]`).forEach((el) => el.remove());
  }

  function applyMasks() {
    clearMasks();
    for (const rect of sensitiveRects()) {
      const mask = document.createElement("div");
      mask.setAttribute(MASK_ATTR, "true");
      Object.assign(mask.style, {
        position: "fixed",
        left: `${rect.x}px`,
        top: `${rect.y}px`,
        width: `${rect.width}px`,
        height: `${rect.height}px`,
        zIndex: "2147483647",
        background: "#ff00ff",
        pointerEvents: "none"
      });
      document.documentElement.appendChild(mask);
    }
  }

  chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
    if (message?.type === "REPORT") {
      sendResponse({
        href: location.href,
        title: document.title,
        viewport: { width: innerWidth, height: innerHeight },
        visibleText: visibleText(),
        sensitiveRects: sensitiveRects(),
        iframeCount: document.querySelectorAll("iframe").length
      });
      return;
    }
    if (message?.type === "APPLY_MASKS") {
      applyMasks();
      sendResponse({ ok: true });
      return;
    }
    if (message?.type === "CLEAR_MASKS") {
      clearMasks();
      sendResponse({ ok: true });
    }
  });
})();
