import { CaptureAuthority, type CaptureToken, type SessionState } from "./capture-authority.js";

export interface ProtocolEnvelope {
  protocol_version: string;
  message_id: string;
  kind: string;
  session_id?: string;
  recording_epoch?: number;
  payload: Record<string, unknown>;
}

export type BuildResult =
  | { allowed: true; reason: "OK"; message: ProtocolEnvelope }
  | { allowed: false; reason: string; message: null };

export type ApplyControlResult =
  | { accepted: true; reason: "OK" }
  | { accepted: false; reason: string };

export class ExtensionProtocolState {
  readonly authority: CaptureAuthority;
  #protocolVersion: string;
  #messageSequence = 0;
  #handshakeComplete = false;

  constructor({ authority = new CaptureAuthority(), protocolVersion = "1.0" }: { authority?: CaptureAuthority; protocolVersion?: string } = {}) {
    this.authority = authority;
    this.#protocolVersion = protocolVersion;
  }

  get snapshot() {
    return Object.freeze({
      protocolVersion: this.#protocolVersion,
      handshakeComplete: this.#handshakeComplete,
      authority: this.authority.snapshot,
    });
  }

  onTransportConnected(): ProtocolEnvelope {
    this.authority.connect();
    this.#handshakeComplete = false;
    return this.#message(
      "hello",
      {
        extension_version: "0.0.0-dev",
        capabilities: [
          "observation_schema_v1",
          "recording_epoch_v1",
          "redacted_raster_v1",
          "visual_fingerprint_v1",
        ],
      },
      false,
    );
  }

  onTransportDisconnected() {
    this.#handshakeComplete = false;
    return this.authority.disconnect();
  }

  applyHelloAck(message: ProtocolEnvelope): ApplyControlResult {
    if (message.kind !== "hello.ack" || message.protocol_version !== this.#protocolVersion || message.payload.compatible !== true) {
      this.#handshakeComplete = false;
      this.authority.suspendLocalCapture();
      return { accepted: false, reason: "INCOMPATIBLE_HANDSHAKE" };
    }

    const state = asSessionState(message.payload.session_state) ?? "IDLE";
    const sessionId = asNullableString(message.payload.session_id);
    const recordingEpoch = asNullableInteger(message.payload.recording_epoch);
    const applied = this.authority.applyHelperState({ state, sessionId, recordingEpoch });
    if (!applied.accepted) {
      this.#handshakeComplete = false;
      this.authority.suspendLocalCapture();
      return applied;
    }

    this.#handshakeComplete = true;
    return { accepted: true, reason: "OK" };
  }

  buildStartRequest(): BuildResult {
    if (!this.#handshakeComplete) {
      return denied("HANDSHAKE_REQUIRED");
    }
    const state = this.authority.snapshot.sessionState;
    if (state !== "IDLE" && state !== "FINISHED") {
      return denied("INVALID_LOCAL_STATE");
    }
    this.authority.suspendLocalCapture();
    return allowed(this.#message("session.start", {}, false));
  }

  buildPauseRequest(): BuildResult {
    return this.#buildRecordingControl("session.pause");
  }

  buildResumeRequest(): BuildResult {
    if (!this.#handshakeComplete) {
      return denied("HANDSHAKE_REQUIRED");
    }
    const state = this.authority.snapshot.sessionState;
    if (state !== "PAUSED" && state !== "INTERRUPTED") {
      return denied("INVALID_LOCAL_STATE");
    }
    this.authority.suspendLocalCapture();
    return allowed(this.#message("session.resume", {}, true));
  }

  buildStopRequest(): BuildResult {
    if (!this.#handshakeComplete) {
      return denied("HANDSHAKE_REQUIRED");
    }
    const state = this.authority.snapshot.sessionState;
    if (!(["RECORDING", "PAUSED", "INTERRUPTED"] as SessionState[]).includes(state)) {
      return denied("INVALID_LOCAL_STATE");
    }
    this.authority.suspendLocalCapture();
    return allowed(this.#message("session.stop", {}, true));
  }

  applyControlAck(requestKind: string, message: ProtocolEnvelope): ApplyControlResult {
    if (!this.#handshakeComplete) {
      this.authority.suspendLocalCapture();
      return { accepted: false, reason: "HANDSHAKE_REQUIRED" };
    }
    if (message.kind !== "ack" || message.payload.accepted !== true) {
      this.authority.suspendLocalCapture();
      return { accepted: false, reason: asString(message.payload.reason) ?? "CONTROL_REJECTED" };
    }

    const nextState = expectedAckState(requestKind);
    if (!nextState) {
      this.authority.suspendLocalCapture();
      return { accepted: false, reason: "UNKNOWN_CONTROL_KIND" };
    }
    const helperState = asSessionState(message.payload.state) ?? nextState;
    if (helperState !== nextState) {
      this.authority.suspendLocalCapture();
      return { accepted: false, reason: "UNEXPECTED_HELPER_STATE" };
    }

    const sessionId = asNullableString(message.payload.session_id) ?? this.authority.snapshot.sessionId;
    const recordingEpoch = asNullableInteger(message.payload.recording_epoch);
    const applied = this.authority.applyHelperState({ state: helperState, sessionId, recordingEpoch });
    if (!applied.accepted) {
      this.authority.suspendLocalCapture();
      return applied;
    }
    return { accepted: true, reason: "OK" };
  }

  buildObservationSubmit(
    event: Record<string, unknown> & { session_id: string; recording_epoch: number },
    token: CaptureToken,
  ): BuildResult {
    const tokenCheck = this.authority.validateCaptureToken(token);
    if (!tokenCheck.valid) {
      return denied(tokenCheck.reason);
    }
    if (event.session_id !== token.sessionId || event.recording_epoch !== token.recordingEpoch) {
      return denied("EVENT_AUTHORITY_MISMATCH");
    }
    return allowed(this.#message("observation.submit", { event }, true));
  }

  #buildRecordingControl(kind: "session.pause"): BuildResult {
    if (!this.#handshakeComplete) {
      return denied("HANDSHAKE_REQUIRED");
    }
    if (this.authority.snapshot.sessionState !== "RECORDING") {
      return denied("INVALID_LOCAL_STATE");
    }
    this.authority.suspendLocalCapture();
    return allowed(this.#message(kind, {}, true));
  }

  #message(kind: string, payload: Record<string, unknown>, scoped: boolean): ProtocolEnvelope {
    const message: ProtocolEnvelope = {
      protocol_version: this.#protocolVersion,
      message_id: `msg_${++this.#messageSequence}`,
      kind,
      payload,
    };
    if (scoped) {
      const snapshot = this.authority.snapshot;
      if (snapshot.sessionId) {
        message.session_id = snapshot.sessionId;
      }
      if (snapshot.recordingEpoch !== null) {
        message.recording_epoch = snapshot.recordingEpoch;
      }
    }
    return message;
  }
}

function expectedAckState(kind: string): SessionState | null {
  switch (kind) {
    case "session.start":
    case "session.resume":
      return "RECORDING";
    case "session.pause":
      return "PAUSED";
    case "session.stop":
      return "FINISHED";
    default:
      return null;
  }
}

function asSessionState(value: unknown): SessionState | null {
  return typeof value === "string" && STATES.has(value as SessionState) ? (value as SessionState) : null;
}

function asNullableString(value: unknown): string | null {
  return typeof value === "string" && value.length > 0 ? value : null;
}

function asNullableInteger(value: unknown): number | null {
  return typeof value === "number" && Number.isInteger(value) && value >= 0 ? value : null;
}

function asString(value: unknown): string | null {
  return typeof value === "string" ? value : null;
}

function allowed(message: ProtocolEnvelope): BuildResult {
  return { allowed: true, reason: "OK", message };
}

function denied(reason: string): BuildResult {
  return { allowed: false, reason, message: null };
}

const STATES = new Set<SessionState>(["IDLE", "RECORDING", "PAUSED", "FINISHED", "INTERRUPTED"]);
