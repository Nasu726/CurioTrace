export type SessionState = "IDLE" | "RECORDING" | "PAUSED" | "FINISHED" | "INTERRUPTED";

export interface HelperState {
  state: SessionState;
  sessionId?: string | null;
  recordingEpoch?: number | null;
}

export interface CaptureToken {
  readonly connectionGeneration: number;
  readonly sessionId: string;
  readonly recordingEpoch: number;
}

export interface AuthoritySnapshot {
  readonly helperConnected: boolean;
  readonly connectionGeneration: number;
  readonly sessionState: SessionState;
  readonly sessionId: string | null;
  readonly recordingEpoch: number | null;
  readonly locallySuspended: boolean;
  readonly captureAllowed: boolean;
}

export type ApplyResult =
  | { accepted: true; reason: "OK"; snapshot: AuthoritySnapshot }
  | { accepted: false; reason: string };

export class CaptureAuthority {
  #helperConnected = false;
  #connectionGeneration = 0;
  #sessionState: SessionState = "IDLE";
  #sessionId: string | null = null;
  #recordingEpoch: number | null = null;
  #locallySuspended = true;

  get snapshot(): AuthoritySnapshot {
    return Object.freeze({
      helperConnected: this.#helperConnected,
      connectionGeneration: this.#connectionGeneration,
      sessionState: this.#sessionState,
      sessionId: this.#sessionId,
      recordingEpoch: this.#recordingEpoch,
      locallySuspended: this.#locallySuspended,
      captureAllowed: this.captureAllowed,
    });
  }

  get captureAllowed(): boolean {
    return Boolean(
      this.#helperConnected &&
        !this.#locallySuspended &&
        this.#sessionState === "RECORDING" &&
        this.#sessionId &&
        Number.isInteger(this.#recordingEpoch),
    );
  }

  connect(): AuthoritySnapshot {
    this.#helperConnected = true;
    this.#connectionGeneration += 1;
    this.#sessionState = "IDLE";
    this.#sessionId = null;
    this.#recordingEpoch = null;
    this.#locallySuspended = true;
    return this.snapshot;
  }

  disconnect(): AuthoritySnapshot {
    this.#helperConnected = false;
    this.#connectionGeneration += 1;
    this.#sessionState = "INTERRUPTED";
    this.#sessionId = null;
    this.#recordingEpoch = null;
    this.#locallySuspended = true;
    return this.snapshot;
  }

  suspendLocalCapture(): AuthoritySnapshot {
    this.#locallySuspended = true;
    return this.snapshot;
  }

  applyHelperState({ state, sessionId = null, recordingEpoch = null }: HelperState): ApplyResult {
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
    this.#locallySuspended = state !== "RECORDING";
    return { accepted: true, reason: "OK", snapshot: this.snapshot };
  }

  beginCapture(): { allowed: true; reason: "OK"; token: CaptureToken } | { allowed: false; reason: string; token: null } {
    if (!this.captureAllowed || !this.#sessionId || this.#recordingEpoch === null) {
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

  validateCaptureToken(token: CaptureToken | null | undefined): { valid: true; reason: "OK" } | { valid: false; reason: string } {
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

  finalizeCapture<T extends Record<string, unknown>>(
    token: CaptureToken,
    buildObservation: () => T,
  ):
    | { accepted: true; reason: "OK"; observation: T & { session_id: string; recording_epoch: number } }
    | { accepted: false; reason: string; observation: null } {
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

const KNOWN_STATES = new Set<SessionState>(["IDLE", "RECORDING", "PAUSED", "FINISHED", "INTERRUPTED"]);
