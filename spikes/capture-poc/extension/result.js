const ext = globalThis.browser ?? chrome;

ext.storage.local.get("lastCapturePoc").then(({ lastCapturePoc }) => {
  const meta = document.getElementById("meta");
  const shot = document.getElementById("shot");

  if (!lastCapturePoc) {
    meta.textContent = "No capture result found. Run the PoC from its permission-explanation popup first.";
    return;
  }

  const { screenshot, ...rest } = lastCapturePoc;
  meta.textContent = JSON.stringify(rest, null, 2);

  if (screenshot) {
    const img = document.createElement("img");
    img.src = screenshot;
    img.alt = "Captured active-tab viewport after in-extension redaction";
    shot.appendChild(img);
    return;
  }

  if (lastCapturePoc.screenshotInfo?.discardedUnredacted) {
    shot.textContent = "Screenshot capture succeeded, but DOM-safe redaction was unavailable. The unredacted pixels were inspected only for dimensions and discarded without persistence.";
    return;
  }

  shot.textContent = `No screenshot: ${lastCapturePoc.captureError || "capture produced no persisted image"}`;
});
