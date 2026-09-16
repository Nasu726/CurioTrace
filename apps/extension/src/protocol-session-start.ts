import {
  ExtensionProtocolState,
  type ApplyControlResult,
  type ProtocolEnvelope,
} from "./protocol-client.js";
import type { SessionStartPort, SessionStartResult } from "./permission-controller.js";

export interface ControlTransport {
  send(message: ProtocolEnvelope): Promise<ProtocolEnvelope>;
}

// Connects the permission gate to the helper-authoritative session protocol.
// PermissionStartController owns *whether* Start may be attempted; this adapter
// owns building/sending/applying exactly one helper session.start transaction.
export class ProtocolSessionStartPort implements SessionStartPort {
  #protocol: ExtensionProtocolState;
  #transport: ControlTransport;

  constructor({ protocol, transport }: { protocol: ExtensionProtocolState; transport: ControlTransport }) {
    this.#protocol = protocol;
    this.#transport = transport;
  }

  async startSession(): Promise<SessionStartResult> {
    const request = this.#protocol.buildStartRequest();
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

    return normalize(this.#protocol.applyControlAck("session.start", response));
  }
}

function normalize(result: ApplyControlResult): SessionStartResult {
  if (result.accepted) {
    return { accepted: true, reason: "OK" };
  }
  return { accepted: false, reason: result.reason };
}
