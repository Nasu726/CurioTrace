import type { CaptureToken, SessionState } from "./capture-authority.js";
import { ExtensionProtocolState, type ProtocolEnvelope } from "./protocol-client.js";
import type { ControlTransport } from "./protocol-session-start.js";

export type ObservationSubmitResult =
  | { accepted: true; reason: "OK" }
  | { accepted: false; reason: string };

export class ProtocolObservationPort {
  #protocol: ExtensionProtocolState;
  #transport: ControlTransport;

  constructor({ protocol, transport }: { protocol: ExtensionProtocolState; transport: ControlTransport }) {
    this.#protocol = protocol;
    this.#transport = transport;
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
      this.#protocol.authority.suspendLocalCapture();
      return { accepted: false, reason: "TRANSPORT_ERROR" };
    }

    if (response.kind !== "ack" || typeof response.payload.accepted !== "boolean") {
      this.#protocol.authority.suspendLocalCapture();
      return { accepted: false, reason: "MALFORMED_OBSERVATION_ACK" };
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
    } else if (AUTHORITY_FAILURES.has(reason)) {
      this.#protocol.authority.suspendLocalCapture();
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
const AUTHORITY_FAILURES = new Set([
  "NOT_RECORDING",
  "UNKNOWN_SESSION",
  "STALE_EPOCH",
  "STORE_ERROR",
  "STORE_NOT_CONFIGURED",
  "STORE_UNAVAILABLE",
  "STATE_PERSISTENCE_ERROR",
]);
