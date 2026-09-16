const ext = globalThis.browser ?? chrome;
const run = document.getElementById("run");
const status = document.getElementById("status");
const SESSION_HOSTS = ["http://*/*", "https://*/*"];

run.addEventListener("click", async () => {
  run.disabled = true;
  status.textContent = "Requesting website-access permission…";

  try {
    const alreadyGranted = await ext.permissions.contains({ origins: SESSION_HOSTS });
    const granted = alreadyGranted || await ext.permissions.request({ origins: SESSION_HOSTS });

    if (!granted) {
      status.textContent = "Permission was not granted. CurioTrace did not start the recording test.";
      run.disabled = false;
      return;
    }

    status.textContent = "Permission granted. Starting capture test…";
    const result = await ext.runtime.sendMessage({ type: "RUN_CAPTURE_POC" });
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
