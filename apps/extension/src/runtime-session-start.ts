import type { SessionStartPort, SessionStartResult } from "./permission-controller.js";

export interface RuntimeSendMessageAPI {
  sendMessage(message: unknown): Promise<unknown>;
}

export class RuntimeSessionStartPort implements SessionStartPort {
  #runtime: RuntimeSendMessageAPI;

  constructor(runtime: RuntimeSendMessageAPI) {
    this.#runtime = runtime;
  }

  async startSession(): Promise<SessionStartResult> {
    let raw: unknown;
    try {
      raw = await this.#runtime.sendMessage({ kind: "curiotrace.session.start" });
    } catch {
      return { accepted: false, reason: "BACKGROUND_UNAVAILABLE" };
    }
    if (!isRecord(raw) || typeof raw.accepted !== "boolean" || typeof raw.reason !== "string") {
      return { accepted: false, reason: "INVALID_BACKGROUND_RESPONSE" };
    }
    if (raw.accepted) {
      return { accepted: true, reason: "OK" };
    }
    return { accepted: false, reason: raw.reason };
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
