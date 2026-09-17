import type { SessionState } from "./capture-authority.js";
import type { SessionControlAction, SessionControlResult } from "./protocol-session-control.js";
import type { RuntimeSendMessageAPI } from "./runtime-session-start.js";

export type BackgroundStateResult =
  | {
      accepted: true;
      reason: "OK";
      sessionState: SessionState;
      sessionId: string | null;
      recordingEpoch: number | null;
      helperConnected: boolean;
    }
  | { accepted: false; reason: string };

type BackgroundResponse =
  | { accepted: true; reason: string; state?: unknown }
  | { accepted: false; reason: string };

export class RuntimeSessionControlPort {
  #runtime: RuntimeSendMessageAPI;

  constructor(runtime: RuntimeSendMessageAPI) {
    this.#runtime = runtime;
  }

  pause(): Promise<SessionControlResult> {
    return this.#control("pause");
  }

  resume(): Promise<SessionControlResult> {
    return this.#control("resume");
  }

  stop(): Promise<SessionControlResult> {
    return this.#control("stop");
  }

  async state(): Promise<BackgroundStateResult> {
    const raw = await this.#send({ kind: "curiotrace.state.get" });
    if (!raw.accepted) {
      return raw;
    }
    const state = parseProtocolState(raw.state);
    if (!state) {
      return { accepted: false, reason: "INVALID_BACKGROUND_STATE" };
    }
    return { accepted: true, reason: "OK", ...state };
  }

  async #control(action: SessionControlAction): Promise<SessionControlResult> {
    const raw = await this.#send({ kind: `curiotrace.session.${action}` });
    if (!raw.accepted) {
      return { accepted: false, reason: raw.reason };
    }
    return { accepted: true, reason: "OK" };
  }

  async #send(message: unknown): Promise<BackgroundResponse> {
    let raw: unknown;
    try {
      raw = await this.#runtime.sendMessage(message);
    } catch {
      return { accepted: false, reason: "BACKGROUND_UNAVAILABLE" };
    }
    if (!isRecord(raw) || typeof raw.accepted !== "boolean" || typeof raw.reason !== "string") {
      return { accepted: false, reason: "INVALID_BACKGROUND_RESPONSE" };
    }
    if (!raw.accepted) {
      return { accepted: false, reason: raw.reason };
    }
    return { accepted: true, reason: raw.reason, state: raw.state };
  }
}

function parseProtocolState(value: unknown): Omit<Extract<BackgroundStateResult, { accepted: true }>, "accepted" | "reason"> | null {
  if (!isRecord(value) || typeof value.handshakeComplete !== "boolean" || !isRecord(value.authority)) {
    return null;
  }
  const authority = value.authority;
  const sessionState = asSessionState(authority.sessionState);
  if (!sessionState || typeof authority.helperConnected !== "boolean") {
    return null;
  }
  const sessionId = typeof authority.sessionId === "string" && authority.sessionId.length > 0 ? authority.sessionId : null;
  const recordingEpoch =
    typeof authority.recordingEpoch === "number" && Number.isInteger(authority.recordingEpoch) && authority.recordingEpoch >= 0
      ? authority.recordingEpoch
      : null;
  return {
    sessionState,
    sessionId,
    recordingEpoch,
    helperConnected: authority.helperConnected,
  };
}

function asSessionState(value: unknown): SessionState | null {
  return typeof value === "string" && STATES.has(value as SessionState) ? (value as SessionState) : null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

const STATES = new Set<SessionState>(["IDLE", "RECORDING", "PAUSED", "FINISHED", "INTERRUPTED"]);
