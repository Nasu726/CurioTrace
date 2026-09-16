import { CaptureAuthority } from "./capture-authority.mjs";

export class ExtensionProtocolState {
  #authority;
  #protocolVersion;
  #messageSequence = 0;
  #handshakeComplete = false;

  constructor({ authority = new CaptureAuthority(), protocolVersion = "1.0" } = {}) {
    this.#authority = authority;
    this.#protocolVersion = protocolVersion;
  }

  get authority() {
    return this.#authority;
  }

  get snapshot() {
    return Object.freeze({
      protocolVersion: this.#protocolVersion,
      handshakeComplete: this.#handshakeComplete,
      authority: this.#authority.snapshot,
    });
  }

  onTransportConnected() {
    this.#authority.connect();
    this.#handshakeComplete = false;
    return this.#message("hello", {
      extension_version: "reference",
      capabilities: [
        "observation_schema_v1",
        "recording_epoch_v1",
        "redacted_raster_v1",
        "visual_fingerprint_v1",
      ],
    }, { scoped: false });
  }

  onTransportDisconnected() {
    this.#handshakeComplete = false;
    this.#authority.disconnect();
    return this.snapshot;
  }

  applyHelloAck(message) {
    if (message?.kind !== "hello.ack" || message?.protocol_version !== this.#protocolVersion) {
      this.#handshakeComplete = false;
      this.#authority.suspendLocalCapture();
      return { accepted: false, reason: "INCOMPATIBLE_HANDSHAKE" };
    }

    const payload = message.payload ?? {};
    if (payload.compatible !== true) {
      this.#handshakeComplete = false;
      this.#authority.suspendLocalCapture();
      return { accepted: false, reason: "INCOMPATIBLE_HANDSHAKE" };
    }

    const applied = this.#authority.applyHelperState({
      state: payload.session_state ?? "IDLE",
      sessionId: payload.session_id ?? null,
      recordingEpoch: payload.recording_epoch ?? null,
    });

    if (!applied.accepted) {
      this.#handshakeComplete = false;
      this.#authority.suspendLocalCapture();
      return applied;
    }

    this.#handshakeComplete = true;
    return { accepted: true, reason: "OK", snapshot: this.snapshot };
  }

  buildStartRequest() {
    if (!this.#handshakeComplete) {
      return { allowed: false, reason: "HANDSHAKE_REQUIRED", message: null };
    }
    if (this.#authority.snapshot.sessionState !== "IDLE" && this.#authority.snapshot.sessionState !== "FINISHED") {
      return { allowed: false, reason: "INVALID_LOCAL_STATE", message: null };
    }

    this.#authority.suspendLocalCapture();
    return { allowed: true, reason: "OK", message: this.#message("session.start", {}, { scoped: false }) };
  }

  buildPauseRequest() {
    return this.#buildRecordingControl("session.pause");
  }

  buildResumeRequest() {
    if (!this.#handshakeComplete) {
      return { allowed: false, reason: "HANDSHAKE_REQUIRED", message: null };
    }
    const state = this.#authority.snapshot.sessionState;
    if (state !== "PAUSED" && state !== "INTERRUPTED") {
      return { allowed: false, reason: "INVALID_LOCAL_STATE", message: null };
    }
    return { allowed: true, reason: "OK", message: this.#message("session.resume", {}, { scoped: true }) };
  }

  buildStopRequest() {
    const state = this.#authority.snapshot.sessionState;
    if (!this.#handshakeComplete) {
      return { allowed: false, reason: "HANDSHAKE_REQUIRED", message: null };
    }
    if (!["RECORDING", "PAUSED", "INTERRUPTED"].includes(state)) {
      return { allowed: false, reason: "INVALID_LOCAL_STATE", message: null };
    }

    this.#authority.suspendLocalCapture();
    return { allowed: true, reason: "OK", message: this.#message("session.stop", {}, { scoped: true }) };
  }

  applyControlAck(requestKind, message) {
    if (!this.#handshakeComplete) {
      this.#authority.suspendLocalCapture();
      return { accepted: false, reason: "HANDSHAKE_REQUIRED" };
    }

    const payload = message?.payload ?? {};
    if (message?.kind !== "ack" || payload.accepted !== true) {
      // Never restore capture based on an unconfirmed/failed control transition.
      this.#authority.suspendLocalCapture();
      return { accepted: false, reason: payload.reason ?? "CONTROL_REJECTED" };
    }

    const nextState = expectedAckState(requestKind);
    if (!nextState) {
      this.#authority.suspendLocalCapture();
      return { accepted: false, reason: "UNKNOWN_CONTROL_KIND" };
    }

    const sessionId = payload.session_id ?? this.#authority.snapshot.sessionId;
    const recordingEpoch = payload.recording_epoch;
    const helperState = payload.state ?? nextState;

    if (helperState !== nextState) {
      this.#authority.suspendLocalCapture();
      return { accepted: false, reason: "UNEXPECTED_HELPER_STATE" };
    }

    const applied = this.#authority.applyHelperState({
      state: helperState,
      sessionId,
      recordingEpoch,
    });
    if (!applied.accepted) {
      this.#authority.suspendLocalCapture();
      return applied;
    }

    return { accepted: true, reason: "OK", snapshot: this.snapshot };
  }

  buildObservationSubmit(event, captureToken) {
    const tokenCheck = this.#authority.validateCaptureToken(captureToken);
    if (!tokenCheck.valid) {
      return { allowed: false, reason: tokenCheck.reason, message: null };
    }

    if (
      event?.session_id !== captureToken.sessionId ||
      event?.recording_epoch !== captureToken.recordingEpoch
    ) {
      return { allowed: false, reason: "EVENT_AUTHORITY_MISMATCH", message: null };
    }

    return {
      allowed: true,
      reason: "OK",
      message: this.#message("observation.submit", { event }, { scoped: true }),
    };
  }

  #buildRecordingControl(kind) {
    if (!this.#handshakeComplete) {
      return { allowed: false, reason: "HANDSHAKE_REQUIRED", message: null };
    }
    if (this.#authority.snapshot.sessionState !== "RECORDING") {
      return { allowed: false, reason: "INVALID_LOCAL_STATE", message: null };
    }

    // Fail closed before the Native Messaging round trip begins.
    this.#authority.suspendLocalCapture();
    return { allowed: true, reason: "OK", message: this.#message(kind, {}, { scoped: true }) };
  }

  #message(kind, payload, { scoped }) {
    const message = {
      protocol_version: this.#protocolVersion,
      message_id: `ref_${++this.#messageSequence}`,
      kind,
      payload,
    };

    if (scoped) {
      message.session_id = this.#authority.snapshot.sessionId;
      message.recording_epoch = this.#authority.snapshot.recordingEpoch;
    }
    return message;
  }
}

function expectedAckState(kind) {
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
