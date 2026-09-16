import { ExtensionProtocolState, type ApplyControlResult, type ProtocolEnvelope } from "./protocol-client.js";
import {
  NativeMessagingTransport,
  type NativePortLike,
  type NativeRuntimeLike,
  type TransportFailureReason,
} from "./native-messaging-transport.js";
import type { ControlTransport } from "./protocol-session-start.js";

export type HelperConnectResult =
  | { accepted: true; reason: "OK" }
  | {
      accepted: false;
      reason:
        | "ALREADY_CONNECTED"
        | "CONNECT_IN_PROGRESS"
        | "CONNECT_NATIVE_FAILED"
        | "HANDSHAKE_TRANSPORT_ERROR"
        | string;
    };

export class HelperConnectionController implements ControlTransport {
  readonly protocol: ExtensionProtocolState;
  #runtime: NativeRuntimeLike;
  #hostName: string;
  #timeoutMs: number;
  #transport: NativeMessagingTransport | null = null;
  #connecting = false;

  constructor({
    runtime,
    hostName,
    protocol = new ExtensionProtocolState(),
    timeoutMs = 10_000,
  }: {
    runtime: NativeRuntimeLike;
    hostName: string;
    protocol?: ExtensionProtocolState;
    timeoutMs?: number;
  }) {
    if (hostName.length === 0) {
      throw new Error("native host name is required");
    }
    if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) {
      throw new RangeError("timeoutMs must be positive");
    }
    this.#runtime = runtime;
    this.#hostName = hostName;
    this.protocol = protocol;
    this.#timeoutMs = timeoutMs;
  }

  get connected(): boolean {
    return this.#transport !== null && !this.#transport.closed && this.protocol.snapshot.handshakeComplete;
  }

  async connect(): Promise<HelperConnectResult> {
    if (this.#connecting) {
      return { accepted: false, reason: "CONNECT_IN_PROGRESS" };
    }
    if (this.#transport !== null && !this.#transport.closed) {
      return { accepted: false, reason: "ALREADY_CONNECTED" };
    }

    this.#connecting = true;
    let port: NativePortLike;
    try {
      port = this.#runtime.connectNative(this.#hostName);
    } catch {
      this.#connecting = false;
      return { accepted: false, reason: "CONNECT_NATIVE_FAILED" };
    }

    let transport: NativeMessagingTransport;
    transport = new NativeMessagingTransport(port, {
      timeoutMs: this.#timeoutMs,
      onTerminalFailure: (_reason: TransportFailureReason) => {
        if (this.#transport === transport) {
          this.#transport = null;
          this.protocol.onTransportDisconnected();
        }
      },
    });
    this.#transport = transport;

    const hello = this.protocol.onTransportConnected();
    let response: ProtocolEnvelope;
    try {
      response = await transport.send(hello);
    } catch {
      if (this.#transport === transport) {
        transport.close();
      }
      this.#connecting = false;
      return { accepted: false, reason: "HANDSHAKE_TRANSPORT_ERROR" };
    }

    const applied = normalize(this.protocol.applyHelloAck(response));
    if (!applied.accepted) {
      transport.close();
    }
    this.#connecting = false;
    return applied;
  }

  async send(message: ProtocolEnvelope): Promise<ProtocolEnvelope> {
    const transport = this.#transport;
    if (!transport || transport.closed || !this.protocol.snapshot.handshakeComplete) {
      throw new Error("helper transport is not connected and handshaken");
    }
    return transport.send(message);
  }

  disconnect(): void {
    const transport = this.#transport;
    if (!transport) {
      if (this.protocol.snapshot.authority.helperConnected) {
        this.protocol.onTransportDisconnected();
      }
      return;
    }
    transport.close();
  }
}

function normalize(result: ApplyControlResult): HelperConnectResult {
  if (result.accepted) {
    return { accepted: true, reason: "OK" };
  }
  return { accepted: false, reason: result.reason };
}
