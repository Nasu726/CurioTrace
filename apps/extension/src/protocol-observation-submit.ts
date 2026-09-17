import type { CaptureToken, SessionState } from "./capture-authority.js";
import { ExtensionProtocolState, type ProtocolEnvelope } from "./protocol-client.js";
import type { ControlTransport } from "./protocol-session-start.js";

export type ObservationSubmitResult =
  | { accepted: true; reason: "OK" }
  | { accepted: false; reason: string };

export class ProtocolObservationPort {
  #protocol: ExtensionProtocolState;
  #transport: ControlTransport;
  #onTerminalFailure: (reason: string) => void;

  constructor({
    protocol,
    transport,
    onTerminalFailure = () => {},
  }: {
    protocol: ExtensionProtocolState;
    transport: ControlTransport;
    onTerminalFailure?: (reason: string) => void;
  }) {
    this.#protocol = protocol;
    this.#transport = transport;
    this.#onTerminalFailure = onTerminalFailure;
  }

  async submit(
    event: Record<string, unknown> & { session_id: string; recording_epoch: number },
    token: CaptureToken,
  ): Promise<ObservationSubmitResult> {
    const request = this.#protocol.buildObservationSubmit(event, token);
    if (!request.allowed) {
      return { accepted: false, reason: request.reason };
    }

    let response: ProtocolEnvelope;
    try {
      response = await this.#transport.send(request.message);
    } catch {
      return this.#terminalFailure("TRANSPORT_ERROR");
    }

    if (response.kind !== "ack" || typeof response.payload.accepted !== "boolean") {
      return this.#terminalFailure("MALFORMED_OBSERVATION_ACK");
    }
    if (response.payload.accepted) {
      return { accepted: true, reason: "OK" };
    }

    const reason = asString(response.payload.reason) ?? "OBSERVATION_REJECTED";
    const helperState = asSessionState(response.payload.state);
    if (helperState) {
      const current = this.#protocol.authority.snapshot;
      const sessionId = asNullableString(response.payload.session_id) ?? current.sessionId;
      const recordingEpoch = asNullableInteger(response.payload.recording_epoch) ?? current.recordingEpoch;
      const applied = this.#protocol.authority.applyHelperState({
        state: helperState,
        sessionId,
        recordingEpoch,
      });
      if (!applied.accepted) {
        this.#protocol.authority.suspendLocalCapture();
      }
    }

    // Any rejected durable observation is terminal for the current browser-side
    // recording path. Schema/privacy rejection indicates an implementation
    // mismatch; authority/storage rejection indicates stale or unavailable
    // helper state. Continuing would silently create an incomplete session.
    return this.#terminalFailure(reason, false);
  }

  #terminalFailure(reason: string, suspend = true): ObservationSubmitResult {
    if (suspend || this.#protocol.authority.captureAllowed) {
      this.#protocol.authority.suspendLocalCapture();
    }
    try {
      this.#onTerminalFailure(reason);
    } catch {
      // Terminal cleanup is best-effort. Capture authority is already suspended,
      // so a cleanup callback must never reactivate or mask the original failure.
    }
    return { accepted: false, reason };
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

const STATES = new Set<SessionState>(["IDLE", "RECORDING", "PAUSED", "FINISHED", "INTERRUPTED"]);
