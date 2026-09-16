const run = document.getElementById("run");
const status = document.getElementById("status");

run.addEventListener("click", async () => {
  run.disabled = true;
  status.textContent = "Requesting permission…";

  try {
    const result = await chrome.runtime.sendMessage({ type: "RUN_CAPTURE_POC" });
    if (!result?.ok) {
      status.textContent = result?.message || "The recording test did not start.";
      run.disabled = false;
      return;
    }

    status.textContent = "Capture test started. See the result tab.";
    window.close();
  } catch (error) {
    status.textContent = String(error?.message || error);
    run.disabled = false;
  }
});
