import {
  HOST_PERMISSION_EXPLANATION,
  PermissionStartController,
  WebExtensionHostPermissionPort,
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

const api = detectExtensionAPI();
const sessionPanel = element<HTMLElement>("session-panel");
const permissionPanel = element<HTMLElement>("permission-panel");
const heading = element<HTMLElement>("state-heading");
const detail = element<HTMLElement>("state-detail");
const errorMessage = element<HTMLElement>("error-message");
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

if (!api) {
  renderUnavailable("EXTENSION_API_UNAVAILABLE");
} else {
  const permissionPort = new WebExtensionHostPermissionPort(api.permissions);
  const sessionStart = new RuntimeSessionStartPort(api.runtime);
  const sessionControls = new RuntimeSessionControlPort(api.runtime);
  const startFlow = new PermissionStartController({ permissions: permissionPort, session: sessionStart });

  element<HTMLElement>("permission-title").textContent = HOST_PERMISSION_EXPLANATION.title;
  element<HTMLElement>("permission-body").textContent = HOST_PERMISSION_EXPLANATION.body;
  continueButton.textContent = HOST_PERMISSION_EXPLANATION.primaryAction;
  cancelButton.textContent = HOST_PERMISSION_EXPLANATION.secondaryAction;

  startButton.addEventListener("click", () => {
    setBusy(true);
    void startFlow.beginStart().then((result) => handleStartResult(result, sessionControls), () => {
      setBusy(false);
      renderUnavailable("START_FLOW_FAILED");
    });
  });

  continueButton.addEventListener("click", () => {
    // Call immediately in this user-action handler. PermissionStartController
    // calls permissions.request() before its first unrelated await.
    const pending = startFlow.continueAfterExplanation();
    setBusy(true);
    void pending.then((result) => handleStartResult(result, sessionControls), () => {
      setBusy(false);
      renderUnavailable("PERMISSION_FLOW_FAILED");
    });
  });

  cancelButton.addEventListener("click", () => {
    const result = startFlow.cancelExplanation();
    hidePermissionPanel();
    if (result.kind === "NOT_STARTED" && result.reason === "USER_CANCELLED") {
      renderReason("");
    }
    void refreshState(sessionControls);
  });

  pauseButton.addEventListener("click", () => void runControl("pause", sessionControls));
  resumeButton.addEventListener("click", () => void runControl("resume", sessionControls));
  stopButton.addEventListener("click", () => void runControl("stop", sessionControls));
  retryButton.addEventListener("click", () => void refreshState(sessionControls));

  void refreshState(sessionControls);
}

async function handleStartResult(result: StartFlowResult, controls: RuntimeSessionControlPort): Promise<void> {
  setBusy(false);
  if (result.kind === "EXPLANATION_REQUIRED") {
    showPermissionPanel();
    return;
  }
  hidePermissionPanel();
  if (result.kind === "NOT_STARTED") {
    if (result.reason === "PERMISSION_DENIED") {
      renderMessage(HOST_PERMISSION_EXPLANATION.denialMessage, result.reason);
    } else {
      renderMessage("Recording was not started.", result.detail ?? result.reason);
    }
  }
  await refreshState(controls, result.kind === "NOT_STARTED");
}

async function runControl(action: Exclude<PopupAction, "start">, controls: RuntimeSessionControlPort): Promise<void> {
  setBusy(true);
  renderReason("");
  const result = await controls[action]();
  setBusy(false);
  if (!result.accepted) {
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
  hidePermissionPanel();
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
  hidePermissionPanel();
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

function showPermissionPanel(): void {
  sessionPanel.hidden = true;
  permissionPanel.hidden = false;
  setBusy(false);
  renderReason("");
}

function hidePermissionPanel(): void {
  permissionPanel.hidden = true;
  sessionPanel.hidden = false;
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
  reasonCode.textContent = reason;
}

function setBusy(busy: boolean): void {
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
