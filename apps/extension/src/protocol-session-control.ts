import { ExtensionProtocolState, type BuildResult, type ProtocolEnvelope } from "./protocol-client.js";
import type { ControlTransport } from "./protocol-session-start.js";

export type SessionControlAction = "pause" | "resume" | "stop";

export type SessionControlResult =
  | { accepted: true; reason: "OK" }
  | { accepted: false; reason: string };

export class ProtocolSessionControlPort {
  #protocol: ExtensionProtocolState;
  #transport: ControlTransport;

  constructor({ protocol, transport }: { protocol: ExtensionProtocolState; transport: ControlTransport }) {
    this.#protocol = protocol;
    this.#transport = transport;
  }

  pause(): Promise<SessionControlResult> {
    return this.#execute("session.pause", this.#protocol.buildPauseRequest());
  }

  resume(): Promise<SessionControlResult> {
    return this.#execute("session.resume", this.#protocol.buildResumeRequest());
  }

  stop(): Promise<SessionControlResult> {
    return this.#execute("session.stop", this.#protocol.buildStopRequest());
  }

  async #execute(requestKind: string, built: BuildResult): Promise<SessionControlResult> {
    if (!built.allowed) {
      return { accepted: false, reason: built.reason };
    }

    let response: ProtocolEnvelope;
    try {
      response = await this.#transport.send(built.message);
    } catch {
      this.#protocol.authority.suspendLocalCapture();
      return { accepted: false, reason: "TRANSPORT_ERROR" };
    }

    const applied = this.#protocol.applyControlAck(requestKind, response);
    if (!applied.accepted) {
      return { accepted: false, reason: applied.reason };
    }
    return { accepted: true, reason: "OK" };
  }
}
