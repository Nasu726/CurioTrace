import {
  HOST_PERMISSION_EXPLANATION,
  PermissionStartController,
  REQUIRED_HOST_ORIGINS,
  WebExtensionHostPermissionPort,
  type HostPermissionPort,
  type StartFlowResult,
  type WebExtensionPermissionsAPI,
} from "./permission-controller.js";
import { popupViewForState, type PopupAction } from "./popup-view-model.js";
import { RuntimeSessionControlPort, type BackgroundStateResult } from "./runtime-session-control.js";
import { RuntimeSessionStartPort, type RuntimeSendMessageAPI } from "./runtime-session-start.js";

interface PopupExtensionAPI {
  runtime: RuntimeSendMessageAPI;
  permissions: WebExtensionPermissionsAPI;
}

type PermissionPanelMode = "start" | "resume";

const RESUME_PERMISSION_EXPLANATION = Object.freeze({
  title: "Restore website access to resume",
  body:
    "Website access is no longer available. CurioTrace stopped browsing observation and cannot resume this session until access is restored.",
  primaryAction: "Restore website access",
  secondaryAction: "Keep paused",
  denialMessage: "The session was not resumed because the required website access was not granted.",
});

const api = detectExtensionAPI();
const shell = element<HTMLElement>("popup-shell");
const sessionPanel = element<HTMLElement>("session-panel");
const permissionPanel = element<HTMLElement>("permission-panel");
const permissionTitle = element<HTMLElement>("permission-title");
const permissionBody = element<HTMLElement>("permission-body");
const permissionPoints = element<HTMLUListElement>("permission-points");
const heading = element<HTMLElement>("state-heading");
const detail = element<HTMLElement>("state-detail");
const errorMessage = element<HTMLElement>("error-message");
const busyStatus = element<HTMLElement>("busy-status");
const technicalDetails = element<HTMLDetailsElement>("technical-details");
const reasonCode = element<HTMLElement>("reason-code");
const statusDot = element<HTMLElement>("status-dot");
const actions = element<HTMLElement>("session-actions");
const startButton = element<HTMLButtonElement>("start-button");
const pauseButton = element<HTMLButtonElement>("pause-button");
const resumeButton = element<HTMLButtonElement>("resume-button");
const stopButton = element<HTMLButtonElement>("stop-button");
const retryButton = element<HTMLButtonElement>("retry-button");
const continueButton = element<HTMLButtonElement>("continue-button");
const cancelButton = element<HTMLButtonElement>("cancel-button");

const allButtons = [startButton, pauseButton, resumeButton, stopButton, retryButton, continueButton, cancelButton];
let permissionMode: PermissionPanelMode | null = null;
let permissionTrigger: HTMLElement | null = null;

if (!api) {
  renderUnavailable("EXTENSION_API_UNAVAILABLE");
} else {
  const permissionPort = new WebExtensionHostPermissionPort(api.permissions);
  const sessionStart = new RuntimeSessionStartPort(api.runtime);
  const sessionControls = new RuntimeSessionControlPort(api.runtime);
  const startFlow = new PermissionStartController({ permissions: permissionPort, session: sessionStart });

  startButton.addEventListener("click", () => {
    setBusy(true);
    void startFlow.beginStart().then((result) => handleStartResult(result, sessionControls), () => {
      setBusy(false);
      renderUnavailable("START_FLOW_FAILED");
    });
  });

  continueButton.addEventListener("click", () => {
    if (permissionMode === "start") {
      // Call immediately in this user-action handler. PermissionStartController
      // calls permissions.request() before its first unrelated await.
      const pending = startFlow.continueAfterExplanation();
      setBusy(true);
      void pending.then((result) => handleStartResult(result, sessionControls), () => {
        setBusy(false);
        renderUnavailable("PERMISSION_FLOW_FAILED");
      });
      return;
    }

    if (permissionMode === "resume") {
      // Keep the browser permission request directly attached to this click.
      const pending = permissionPort.request(REQUIRED_HOST_ORIGINS);
      setBusy(true);
      void pending.then(
        (granted) => continueResumeAfterPermission(granted, permissionPort, sessionControls),
        () => {
          setBusy(false);
          hidePermissionPanel({ restoreFocus: true });
          renderMessage("Website access could not be requested.", "PERMISSION_REQUEST_FAILED");
        },
      );
    }
  });

  cancelButton.addEventListener("click", () => {
    if (permissionMode === "start") {
      const result = startFlow.cancelExplanation();
      if (result.kind === "NOT_STARTED" && result.reason === "USER_CANCELLED") {
        renderReason("");
      }
    }
    hidePermissionPanel({ restoreFocus: true });
    void refreshState(sessionControls);
  });

  pauseButton.addEventListener("click", () => void runControl("pause", sessionControls));
  resumeButton.addEventListener("click", () => void beginResume(permissionPort, sessionControls));
  stopButton.addEventListener("click", () => void runControl("stop", sessionControls));
  retryButton.addEventListener("click", () => void refreshState(sessionControls));

  void refreshState(sessionControls);
}

async function handleStartResult(result: StartFlowResult, controls: RuntimeSessionControlPort): Promise<void> {
  setBusy(false);
  if (result.kind === "EXPLANATION_REQUIRED") {
    showPermissionPanel("start", startButton);
    return;
  }
  hidePermissionPanel({ restoreFocus: false });
  if (result.kind === "NOT_STARTED") {
    if (result.reason === "PERMISSION_DENIED") {
      renderMessage(HOST_PERMISSION_EXPLANATION.denialMessage, result.reason);
    } else {
      renderMessage("Recording was not started.", result.detail ?? result.reason);
    }
  }
  await refreshState(controls, result.kind === "NOT_STARTED");
}

async function beginResume(
  permissions: HostPermissionPort,
  controls: RuntimeSessionControlPort,
): Promise<void> {
  setBusy(true);
  renderReason("");
  let granted: boolean;
  try {
    granted = await permissions.contains(REQUIRED_HOST_ORIGINS);
  } catch {
    setBusy(false);
    renderMessage("Website access could not be checked. The session remains paused.", "PERMISSION_CHECK_FAILED");
    return;
  }
  setBusy(false);
  if (!granted) {
    showPermissionPanel("resume", resumeButton);
    return;
  }
  await runControl("resume", controls);
}

async function continueResumeAfterPermission(
  granted: boolean,
  permissions: HostPermissionPort,
  controls: RuntimeSessionControlPort,
): Promise<void> {
  if (!granted) {
    setBusy(false);
    hidePermissionPanel({ restoreFocus: true });
    renderMessage(RESUME_PERMISSION_EXPLANATION.denialMessage, "PERMISSION_DENIED");
    return;
  }

  try {
    if (!(await permissions.contains(REQUIRED_HOST_ORIGINS))) {
      setBusy(false);
      hidePermissionPanel({ restoreFocus: true });
      renderMessage(RESUME_PERMISSION_EXPLANATION.denialMessage, "PERMISSION_NOT_EFFECTIVE");
      return;
    }
  } catch {
    setBusy(false);
    hidePermissionPanel({ restoreFocus: true });
    renderMessage("Website access could not be verified. The session remains paused.", "PERMISSION_CHECK_FAILED");
    return;
  }

  hidePermissionPanel({ restoreFocus: false });
  setBusy(false);
  await runControl("resume", controls);
}

async function runControl(action: Exclude<PopupAction, "start">, controls: RuntimeSessionControlPort): Promise<void> {
  setBusy(true);
  renderReason("");
  const result = await controls[action]();
  setBusy(false);
  if (!result.accepted) {
    if (action === "resume" && result.reason === "HOST_PERMISSION_REQUIRED") {
      showPermissionPanel("resume", resumeButton);
      return;
    }
    renderMessage("The session state could not be changed.", result.reason);
  }
  await refreshState(controls, !result.accepted);
}

async function refreshState(controls: RuntimeSessionControlPort, preserveMessage = false): Promise<void> {
  setBusy(true);
  if (!preserveMessage) {
    clearMessage();
  }
  const result = await controls.state();
  setBusy(false);
  renderState(result, preserveMessage);
}

function renderState(result: BackgroundStateResult, preserveMessage: boolean): void {
  hidePermissionPanel({ restoreFocus: false });
  if (!result.accepted) {
    renderUnavailable(result.reason);
    return;
  }

  const view = popupViewForState(result.sessionState);
  heading.textContent = view.heading;
  detail.textContent = view.detail;
  statusDot.classList.toggle("recording", view.recording);
  actions.hidden = false;
  for (const action of ["start", "pause", "resume", "stop"] as const) {
    buttonFor(action).hidden = !view.actions.includes(action);
  }
  retryButton.hidden = true;
  if (!preserveMessage) {
    clearMessage();
  }
}

function renderUnavailable(reason: string): void {
  hidePermissionPanel({ restoreFocus: false });
  heading.textContent = "Helper unavailable";
  detail.textContent = "CurioTrace is not recording. Check the local helper installation and try again.";
  statusDot.classList.remove("recording");
  actions.hidden = false;
  for (const action of ["start", "pause", "resume", "stop"] as const) {
    buttonFor(action).hidden = true;
  }
  retryButton.hidden = false;
  renderReason(reason);
}

function showPermissionPanel(mode: PermissionPanelMode, trigger: HTMLElement): void {
  permissionMode = mode;
  permissionTrigger = trigger;
  sessionPanel.hidden = true;
  permissionPanel.hidden = false;
  setBusy(false);
  renderReason("");

  if (mode === "start") {
    permissionTitle.textContent = HOST_PERMISSION_EXPLANATION.title;
    permissionBody.textContent = HOST_PERMISSION_EXPLANATION.body;
    continueButton.textContent = HOST_PERMISSION_EXPLANATION.primaryAction;
    cancelButton.textContent = HOST_PERMISSION_EXPLANATION.secondaryAction;
  } else {
    permissionTitle.textContent = RESUME_PERMISSION_EXPLANATION.title;
    permissionBody.textContent = RESUME_PERMISSION_EXPLANATION.body;
    continueButton.textContent = RESUME_PERMISSION_EXPLANATION.primaryAction;
    cancelButton.textContent = RESUME_PERMISSION_EXPLANATION.secondaryAction;
  }
  renderPermissionPoints();
  permissionTitle.focus({ preventScroll: true });
}

function hidePermissionPanel({ restoreFocus }: { restoreFocus: boolean }): void {
  const trigger = permissionTrigger;
  permissionPanel.hidden = true;
  sessionPanel.hidden = false;
  permissionMode = null;
  permissionTrigger = null;
  if (restoreFocus && trigger && !trigger.hidden && !trigger.disabled) {
    trigger.focus({ preventScroll: true });
  }
}

function renderPermissionPoints(): void {
  const items = HOST_PERMISSION_EXPLANATION.points.map((point) => {
    const item = document.createElement("li");
    item.textContent = point;
    return item;
  });
  permissionPoints.replaceChildren(...items);
}

function renderMessage(message: string, reason: string): void {
  errorMessage.hidden = false;
  errorMessage.textContent = message;
  renderReason(reason);
}

function clearMessage(): void {
  errorMessage.hidden = true;
  errorMessage.textContent = "";
  renderReason("");
}

function renderReason(reason: string): void {
  technicalDetails.hidden = reason.length === 0;
  if (reason.length === 0) {
    technicalDetails.open = false;
  }
  reasonCode.textContent = reason;
}

function setBusy(busy: boolean): void {
  shell.setAttribute("aria-busy", String(busy));
  busyStatus.hidden = !busy;
  for (const button of allButtons) {
    button.disabled = busy;
  }
}

function buttonFor(action: PopupAction): HTMLButtonElement {
  switch (action) {
    case "start":
      return startButton;
    case "pause":
      return pauseButton;
    case "resume":
      return resumeButton;
    case "stop":
      return stopButton;
  }
}

function detectExtensionAPI(): PopupExtensionAPI | null {
  const scope = globalThis as typeof globalThis & {
    browser?: PopupExtensionAPI;
    chrome?: PopupExtensionAPI;
  };
  return scope.browser ?? scope.chrome ?? null;
}

function element<T extends HTMLElement>(id: string): T {
  const found = document.getElementById(id);
  if (!found) {
    throw new Error(`missing popup element: ${id}`);
  }
  return found as T;
}
