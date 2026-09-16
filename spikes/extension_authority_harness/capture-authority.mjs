export class CaptureAuthority {
  #helperConnected = false;
  #connectionGeneration = 0;
  #sessionState = "IDLE";
  #sessionId = null;
  #recordingEpoch = null;

  get snapshot() {
    return Object.freeze({
      helperConnected: this.#helperConnected,
      connectionGeneration: this.#connectionGeneration,
      sessionState: this.#sessionState,
      sessionId: this.#sessionId,
      recordingEpoch: this.#recordingEpoch,
      captureAllowed: this.captureAllowed,
    });
  }

  get captureAllowed() {
    return Boolean(
      this.#helperConnected &&
      this.#sessionState === "RECORDING" &&
      this.#sessionId &&
      Number.isInteger(this.#recordingEpoch)
    );
  }

  connect() {
    this.#helperConnected = true;
    this.#connectionGeneration += 1;
    // A new transport connection carries no implicit recording authority.
    this.#sessionState = "IDLE";
    this.#sessionId = null;
    this.#recordingEpoch = null;
    return this.snapshot;
  }

  disconnect() {
    this.#helperConnected = false;
    this.#connectionGeneration += 1;
    this.#sessionState = "INTERRUPTED";
    this.#sessionId = null;
    this.#recordingEpoch = null;
    return this.snapshot;
  }

  applyHelperState({ state, sessionId = null, recordingEpoch = null }) {
    if (!this.#helperConnected) {
      return { accepted: false, reason: "HELPER_DISCONNECTED" };
    }

    if (!KNOWN_STATES.has(state)) {
      return { accepted: false, reason: "UNKNOWN_STATE" };
    }

    if (state === "RECORDING") {
      if (!sessionId || !Number.isInteger(recordingEpoch)) {
        return { accepted: false, reason: "INCOMPLETE_RECORDING_AUTHORITY" };
      }
    }

    this.#sessionState = state;
    this.#sessionId = sessionId;
    this.#recordingEpoch = recordingEpoch;
    return { accepted: true, reason: "OK", snapshot: this.snapshot };
  }

  beginCapture() {
    if (!this.captureAllowed) {
      return { allowed: false, reason: "CAPTURE_NOT_AUTHORIZED", token: null };
    }

    return {
      allowed: true,
      reason: "OK",
      token: Object.freeze({
        connectionGeneration: this.#connectionGeneration,
        sessionId: this.#sessionId,
        recordingEpoch: this.#recordingEpoch,
      }),
    };
  }

  validateCaptureToken(token) {
    if (!token || !this.captureAllowed) {
      return { valid: false, reason: "CAPTURE_NOT_AUTHORIZED" };
    }

    if (token.connectionGeneration !== this.#connectionGeneration) {
      return { valid: false, reason: "STALE_CONNECTION" };
    }
    if (token.sessionId !== this.#sessionId) {
      return { valid: false, reason: "STALE_SESSION" };
    }
    if (token.recordingEpoch !== this.#recordingEpoch) {
      return { valid: false, reason: "STALE_EPOCH" };
    }

    return { valid: true, reason: "OK" };
  }

  finalizeCapture(token, buildObservation) {
    const validation = this.validateCaptureToken(token);
    if (!validation.valid) {
      return { accepted: false, reason: validation.reason, observation: null };
    }

    const payload = buildObservation();
    return {
      accepted: true,
      reason: "OK",
      observation: {
        ...payload,
        session_id: token.sessionId,
        recording_epoch: token.recordingEpoch,
      },
    };
  }
}

const KNOWN_STATES = new Set([
  "IDLE",
  "RECORDING",
  "PAUSED",
  "FINISHED",
  "INTERRUPTED",
]);
