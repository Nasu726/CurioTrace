import { HelperConnectionController } from "./helper-connection.js";
import {
  REQUIRED_HOST_ORIGINS,
  type HostPermissionPort,
  type SessionStartResult,
} from "./permission-controller.js";
import { ProtocolSessionStartPort } from "./protocol-session-start.js";

export type BackgroundRequest =
  | { kind: "curiotrace.helper.ensure-connected" }
  | { kind: "curiotrace.session.start" }
  | { kind: "curiotrace.state.get" };

export type BackgroundResponse =
  | { accepted: true; reason: "OK"; state?: unknown }
  | { accepted: false; reason: string };

export interface ExtensionMessageSender {
  id?: string;
  url?: string;
  tab?: unknown;
}

export class BackgroundSessionBroker {
  #runtimeId: string;
  #permissions: Pick<HostPermissionPort, "contains">;
  #helper: HelperConnectionController;
  #startPort: ProtocolSessionStartPort;

  constructor({
    runtimeId,
    permissions,
    helper,
  }: {
    runtimeId: string;
    permissions: Pick<HostPermissionPort, "contains">;
    helper: HelperConnectionController;
  }) {
    if (!runtimeId) {
      throw new Error("runtimeId is required");
    }
    this.#runtimeId = runtimeId;
    this.#permissions = permissions;
    this.#helper = helper;
    this.#startPort = new ProtocolSessionStartPort({
      protocol: helper.protocol,
      transport: helper,
    });
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
      case "curiotrace.state.get":
        return {
          accepted: true,
          reason: "OK",
          state: this.#helper.protocol.snapshot,
        };
      default:
        return { accepted: false, reason: "UNKNOWN_BACKGROUND_REQUEST" };
    }
  }

  async #startSession(): Promise<BackgroundResponse> {
    let granted: boolean;
    try {
      granted = await this.#permissions.contains(REQUIRED_HOST_ORIGINS);
    } catch {
      return { accepted: false, reason: "PERMISSION_CHECK_FAILED" };
    }
    if (!granted) {
      return { accepted: false, reason: "HOST_PERMISSION_REQUIRED" };
    }

    const connected = await this.#ensureHelperConnected();
    if (!connected.accepted) {
      return connected;
    }

    return normalizeStart(await this.#startPort.startSession());
  }

  async #ensureHelperConnected(): Promise<BackgroundResponse> {
    if (this.#helper.connected) {
      return { accepted: true, reason: "OK" };
    }
    const result = await this.#helper.connect();
    if (!result.accepted) {
      return { accepted: false, reason: result.reason };
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

function normalizeStart(result: SessionStartResult): BackgroundResponse {
  if (result.accepted) {
    return { accepted: true, reason: "OK" };
  }
  return { accepted: false, reason: result.reason };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
