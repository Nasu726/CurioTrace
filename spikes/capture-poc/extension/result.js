chrome.storage.local.get("lastCapturePoc").then(({ lastCapturePoc }) => {
  const meta = document.getElementById("meta");
  const shot = document.getElementById("shot");

  if (!lastCapturePoc) {
    meta.textContent = "No capture result found. Click the extension action on a test page first.";
    return;
  }

  const { screenshot, ...rest } = lastCapturePoc;
  meta.textContent = JSON.stringify(rest, null, 2);

  if (screenshot) {
    const img = document.createElement("img");
    img.src = screenshot;
    img.alt = "Captured active-tab viewport";
    shot.appendChild(img);
  } else {
    shot.textContent = `No screenshot: ${lastCapturePoc.captureError || "unknown error"}`;
  }
});
