import { HelperConnectionController } from "./helper-connection.js";
import {
  REQUIRED_HOST_ORIGINS,
  type HostPermissionPort,
  type SessionStartResult,
} from "./permission-controller.js";
import {
  ProtocolSessionControlPort,
  type SessionControlAction,
  type SessionControlResult,
} from "./protocol-session-control.js";
import { ProtocolSessionStartPort } from "./protocol-session-start.js";

export type BackgroundRequest =
  | { kind: "curiotrace.helper.ensure-connected" }
  | { kind: "curiotrace.session.start" }
  | { kind: "curiotrace.session.pause" }
  | { kind: "curiotrace.session.resume" }
  | { kind: "curiotrace.session.stop" }
  | { kind: "curiotrace.state.get" };

export type BackgroundResponse =
  | { accepted: true; reason: "OK"; state?: unknown }
  | { accepted: false; reason: string; state?: unknown };

export interface ExtensionMessageSender {
  id?: string;
  url?: string;
  tab?: unknown;
}

export interface RecordingActivationObserver {
  syncCurrentActiveView(
    reason: "session_start" | "session_resume",
  ): Promise<{ accepted: true; reason: "OK" } | { accepted: false; reason: string }>;
  suspendObservation(): void;
}

export class BackgroundSessionBroker {
  #runtimeId: string;
  #permissions: Pick<HostPermissionPort, "contains">;
  #helper: HelperConnectionController;
  #recordingObserver: RecordingActivationObserver | null;
  #startPort: ProtocolSessionStartPort;
  #controlPort: ProtocolSessionControlPort;

  constructor({
    runtimeId,
    permissions,
    helper,
    recordingObserver = null,
  }: {
    runtimeId: string;
    permissions: Pick<HostPermissionPort, "contains">;
    helper: HelperConnectionController;
    recordingObserver?: RecordingActivationObserver | null;
  }) {
    if (!runtimeId) {
      throw new Error("runtimeId is required");
    }
    this.#runtimeId = runtimeId;
    this.#permissions = permissions;
    this.#helper = helper;
    this.#recordingObserver = recordingObserver;
    this.#startPort = new ProtocolSessionStartPort({ protocol: helper.protocol, transport: helper });
    this.#controlPort = new ProtocolSessionControlPort({ protocol: helper.protocol, transport: helper });
  }

  async handle(request: unknown, sender: ExtensionMessageSender): Promise<BackgroundResponse> {
    if (!this.#trustedExtensionPage(sender)) {
      return { accepted: false, reason: "UNTRUSTED_MESSAGE_SENDER" };
    }
    if (!isRecord(request) || typeof request.kind !== "string") {
      return { accepted: false, reason: "INVALID_BACKGROUND_REQUEST" };
    }

    switch (request.kind) {
      case "curiotrace.helper.ensure-connected":
        return this.#ensureHelperConnected();
      case "curiotrace.session.start":
        return this.#startSession();
      case "curiotrace.session.pause":
        return this.#control("pause");
      case "curiotrace.session.resume":
        return this.#control("resume");
      case "curiotrace.session.stop":
        return this.#control("stop");
      case "curiotrace.state.get":
        return this.#state();
      default:
        return { accepted: false, reason: "UNKNOWN_BACKGROUND_REQUEST" };
    }
  }

  async #startSession(): Promise<BackgroundResponse> {
    const permission = await this.#requiredHostPermissionStatus();
    if (!permission.accepted) {
      return permission;
    }

    const connected = await this.#ensureHelperConnected();
    if (!connected.accepted) {
      return connected;
    }
    const started = await this.#startPort.startSession();
    if (!started.accepted) {
      return normalize(started, this.#helper.protocol.snapshot);
    }
    return this.#finishRecordingActivation("session_start");
  }

  async #control(action: SessionControlAction): Promise<BackgroundResponse> {
    if (action === "pause" || action === "stop") {
      // Stop browser event delivery at the user-control boundary, before any
      // async helper round trip can expose additional URL/title metadata.
      this.#recordingObserver?.suspendObservation();
    }

    if (action === "resume") {
      const permission = await this.#requiredHostPermissionStatus();
      if (!permission.accepted) {
        return permission;
      }
    }

    // A Pause/Stop/Resume command must not cause a newly connected helper that
    // reports RECORDING to reacquire browser observations before the requested
    // control transition has been applied.
    const connected = await this.#ensureHelperConnected({ recoverRecording: false });
    if (!connected.accepted) {
      return connected;
    }

    let result: SessionControlResult;
    switch (action) {
      case "pause":
        result = await this.#controlPort.pause();
        break;
      case "resume":
        result = await this.#controlPort.resume();
        break;
      case "stop":
        result = await this.#controlPort.stop();
        break;
    }
    if (!result.accepted) {
      return normalize(result, this.#helper.protocol.snapshot);
    }
    if (action === "resume") {
      return this.#finishRecordingActivation("session_resume");
    }
    return normalize(result, this.#helper.protocol.snapshot);
  }

  async #finishRecordingActivation(
    reason: "session_start" | "session_resume",
  ): Promise<BackgroundResponse> {
    if (!this.#recordingObserver) {
      this.#helper.disconnect();
      return {
        accepted: false,
        reason: "COLLECTOR_NOT_CONFIGURED",
        state: this.#helper.protocol.snapshot,
      };
    }
    const observed = await this.#recordingObserver.syncCurrentActiveView(reason);
    if (!observed.accepted) {
      this.#recordingObserver.suspendObservation();
      this.#helper.disconnect();
      return {
        accepted: false,
        reason: observed.reason,
        state: this.#helper.protocol.snapshot,
      };
    }
    return {
      accepted: true,
      reason: "OK",
      state: this.#helper.protocol.snapshot,
    };
  }

  async #state(): Promise<BackgroundResponse> {
    const connected = await this.#ensureHelperConnected();
    if (!connected.accepted) {
      return {
        ...connected,
        state: this.#helper.protocol.snapshot,
      };
    }
    return {
      accepted: true,
      reason: "OK",
      state: this.#helper.protocol.snapshot,
    };
  }

  async #ensureHelperConnected(
    { recoverRecording = true }: { recoverRecording?: boolean } = {},
  ): Promise<BackgroundResponse> {
    if (this.#helper.connected) {
      return { accepted: true, reason: "OK" };
    }
    const result = await this.#helper.connect();
    if (!result.accepted) {
      return { accepted: false, reason: result.reason };
    }

    if (recoverRecording && this.#helper.protocol.snapshot.authority.sessionState === "RECORDING") {
      const permission = await this.#requiredHostPermissionStatus();
      if (!permission.accepted) {
        this.#recordingObserver?.suspendObservation();
        this.#helper.disconnect();
        return permission;
      }
      // A background/service-worker recovery creates a fresh observation
      // segment. Reusing the resume snapshot semantics is conservative: the
      // unobserved interval is never treated as continuous exposure.
      return this.#finishRecordingActivation("session_resume");
    }
    return { accepted: true, reason: "OK" };
  }

  async #requiredHostPermissionStatus(): Promise<BackgroundResponse> {
    let granted: boolean;
    try {
      granted = await this.#permissions.contains(REQUIRED_HOST_ORIGINS);
    } catch {
      return { accepted: false, reason: "PERMISSION_CHECK_FAILED" };
    }
    if (!granted) {
      return { accepted: false, reason: "HOST_PERMISSION_REQUIRED" };
    }
    return { accepted: true, reason: "OK" };
  }

  #trustedExtensionPage(sender: ExtensionMessageSender): boolean {
    if (sender.id !== this.#runtimeId || sender.tab !== undefined) {
      return false;
    }
    if (typeof sender.url !== "string") {
      return false;
    }
    return sender.url.startsWith("chrome-extension://") || sender.url.startsWith("moz-extension://");
  }
}

function normalize(
  result: SessionStartResult | SessionControlResult,
  state: unknown,
): BackgroundResponse {
  if (result.accepted) {
    return { accepted: true, reason: "OK", state };
  }
  return { accepted: false, reason: result.reason, state };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
