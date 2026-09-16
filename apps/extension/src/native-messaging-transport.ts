import type { ProtocolEnvelope } from "./protocol-client.js";
import type { ControlTransport } from "./protocol-session-start.js";

export interface WebExtensionEvent<Listener extends (...args: never[]) => void> {
  addListener(listener: Listener): void;
  removeListener?(listener: Listener): void;
}

export interface NativePortLike {
  postMessage(message: unknown): void;
  disconnect(): void;
  onMessage: WebExtensionEvent<(message: unknown) => void>;
  onDisconnect: WebExtensionEvent<() => void>;
}

export interface NativeRuntimeLike {
  connectNative(application: string): NativePortLike;
}

export type TransportFailureReason =
  | "TRANSPORT_CLOSED"
  | "TRANSPORT_TIMEOUT"
  | "TRANSPORT_POST_FAILED"
  | "MALFORMED_NATIVE_MESSAGE";

export class NativeTransportError extends Error {
  readonly reason: TransportFailureReason;

  constructor(reason: TransportFailureReason) {
    super(reason);
    this.name = "NativeTransportError";
    this.reason = reason;
  }
}

type PendingRequest = {
  resolve: (message: ProtocolEnvelope) => void;
  reject: (error: NativeTransportError) => void;
  timeout: ReturnType<typeof setTimeout>;
};

export class NativeMessagingTransport implements ControlTransport {
  #port: NativePortLike;
  #pending = new Map<string, PendingRequest>();
  #closed = false;
  #timeoutMs: number;
  #onTerminalFailure: (reason: TransportFailureReason) => void;

  constructor(
    port: NativePortLike,
    {
      timeoutMs = 10_000,
      onTerminalFailure = () => {},
    }: {
      timeoutMs?: number;
      onTerminalFailure?: (reason: TransportFailureReason) => void;
    } = {},
  ) {
    if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) {
      throw new RangeError("timeoutMs must be positive");
    }
    this.#port = port;
    this.#timeoutMs = timeoutMs;
    this.#onTerminalFailure = onTerminalFailure;
    this.#port.onMessage.addListener((message) => this.#handleMessage(message));
    this.#port.onDisconnect.addListener(() => this.#failTerminal("TRANSPORT_CLOSED", false));
  }

  get closed(): boolean {
    return this.#closed;
  }

  async send(message: ProtocolEnvelope): Promise<ProtocolEnvelope> {
    if (this.#closed) {
      throw new NativeTransportError("TRANSPORT_CLOSED");
    }
    if (!message.message_id || this.#pending.has(message.message_id)) {
      throw new Error("message_id must be non-empty and unique among pending requests");
    }

    return new Promise<ProtocolEnvelope>((resolve, reject) => {
      const timeout = setTimeout(() => {
        if (!this.#pending.has(message.message_id)) {
          return;
        }
        this.#failTerminal("TRANSPORT_TIMEOUT", true);
      }, this.#timeoutMs);

      this.#pending.set(message.message_id, { resolve, reject, timeout });
      try {
        this.#port.postMessage(message);
      } catch {
        this.#failTerminal("TRANSPORT_POST_FAILED", true);
      }
    });
  }

  close(): void {
    this.#failTerminal("TRANSPORT_CLOSED", true);
  }

  #handleMessage(value: unknown): void {
    const message = parseEnvelope(value);
    if (!message) {
      this.#failTerminal("MALFORMED_NATIVE_MESSAGE", true);
      return;
    }
    const pending = this.#pending.get(message.message_id);
    if (!pending) {
      // Unsolicited/late messages are ignored rather than logged. Future
      // helper-push messages need an explicit protocol capability and handler.
      return;
    }
    clearTimeout(pending.timeout);
    this.#pending.delete(message.message_id);
    pending.resolve(message);
  }

  #failTerminal(reason: TransportFailureReason, disconnectPort: boolean): void {
    if (this.#closed) {
      return;
    }
    this.#closed = true;
    const error = new NativeTransportError(reason);
    for (const pending of this.#pending.values()) {
      clearTimeout(pending.timeout);
      pending.reject(error);
    }
    this.#pending.clear();

    if (disconnectPort) {
      try {
        this.#port.disconnect();
      } catch {
        // The transport is already terminal; never recover optimistically.
      }
    }
    this.#onTerminalFailure(reason);
  }
}

function parseEnvelope(value: unknown): ProtocolEnvelope | null {
  if (!isRecord(value)) {
    return null;
  }
  if (
    typeof value.protocol_version !== "string" ||
    typeof value.message_id !== "string" ||
    value.message_id.length === 0 ||
    typeof value.kind !== "string" ||
    !isRecord(value.payload)
  ) {
    return null;
  }

  const message: ProtocolEnvelope = {
    protocol_version: value.protocol_version,
    message_id: value.message_id,
    kind: value.kind,
    payload: value.payload,
  };
  if (typeof value.session_id === "string") {
    message.session_id = value.session_id;
  }
  if (typeof value.recording_epoch === "number" && Number.isInteger(value.recording_epoch) && value.recording_epoch >= 0) {
    message.recording_epoch = value.recording_epoch;
  }
  return message;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
