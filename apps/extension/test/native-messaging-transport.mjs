import assert from "node:assert/strict";
import test from "node:test";

import { HelperConnectionController } from "../dist/helper-connection.js";
import {
  NativeMessagingTransport,
  NativeTransportError,
} from "../dist/native-messaging-transport.js";

class FakeEvent {
  listeners = [];

  addListener(listener) {
    this.listeners.push(listener);
  }

  removeListener(listener) {
    this.listeners = this.listeners.filter((candidate) => candidate !== listener);
  }

  emit(...args) {
    for (const listener of [...this.listeners]) listener(...args);
  }
}

class FakePort {
  onMessage = new FakeEvent();
  onDisconnect = new FakeEvent();
  sent = [];
  disconnectCalls = 0;
  onPost = null;

  postMessage(message) {
    this.sent.push(message);
    this.onPost?.(message);
  }

  disconnect() {
    this.disconnectCalls += 1;
  }

  emitMessage(message) {
    this.onMessage.emit(message);
  }

  emitDisconnect() {
    this.onDisconnect.emit();
  }
}

function envelope(id = "msg_1", kind = "ack", payload = { accepted: true }) {
  return {
    protocol_version: "1.0",
    message_id: id,
    kind,
    payload,
  };
}

function helloAck(request, { compatible = true, state = "IDLE", sessionId = null, epoch = 0 } = {}) {
  return {
    protocol_version: "1.0",
    message_id: request.message_id,
    kind: "hello.ack",
    payload: {
      compatible,
      session_state: state,
      session_id: sessionId,
      recording_epoch: epoch,
    },
  };
}

test("native transport correlates responses by message_id", async () => {
  const port = new FakePort();
  const transport = new NativeMessagingTransport(port);
  const pending = transport.send(envelope("msg_a", "hello", {}));
  assert.equal(port.sent.length, 1);

  port.emitMessage(envelope("unrelated"));
  port.emitMessage(envelope("msg_a", "hello.ack", { compatible: true }));
  const response = await pending;
  assert.equal(response.message_id, "msg_a");
  assert.equal(transport.closed, false);
});

test("disconnect rejects all pending requests and becomes terminal", async () => {
  const port = new FakePort();
  const failures = [];
  const transport = new NativeMessagingTransport(port, {
    onTerminalFailure: (reason) => failures.push(reason),
  });
  const first = transport.send(envelope("msg_1"));
  const second = transport.send(envelope("msg_2"));

  port.emitDisconnect();
  await assert.rejects(first, (error) => error instanceof NativeTransportError && error.reason === "TRANSPORT_CLOSED");
  await assert.rejects(second, (error) => error instanceof NativeTransportError && error.reason === "TRANSPORT_CLOSED");
  assert.equal(transport.closed, true);
  assert.deepEqual(failures, ["TRANSPORT_CLOSED"]);
  await assert.rejects(transport.send(envelope("msg_3")), /TRANSPORT_CLOSED/);
});

test("timeout is terminal because helper authority may have changed without an ack", async () => {
  const port = new FakePort();
  const failures = [];
  const transport = new NativeMessagingTransport(port, {
    timeoutMs: 15,
    onTerminalFailure: (reason) => failures.push(reason),
  });

  await assert.rejects(
    transport.send(envelope("msg_timeout")),
    (error) => error instanceof NativeTransportError && error.reason === "TRANSPORT_TIMEOUT",
  );
  assert.equal(transport.closed, true);
  assert.equal(port.disconnectCalls, 1);
  assert.deepEqual(failures, ["TRANSPORT_TIMEOUT"]);
});

test("malformed helper message terminates connection without echoing payload", async () => {
  const port = new FakePort();
  const transport = new NativeMessagingTransport(port);
  const pending = transport.send(envelope("msg_bad"));

  port.emitMessage({ message_id: "msg_bad", payload: "not-an-object" });
  await assert.rejects(
    pending,
    (error) => error instanceof NativeTransportError && error.reason === "MALFORMED_NATIVE_MESSAGE",
  );
  assert.equal(port.disconnectCalls, 1);
  assert.equal(transport.closed, true);
});

test("duplicate pending message IDs are rejected without disturbing the original", async () => {
  const port = new FakePort();
  const transport = new NativeMessagingTransport(port);
  const first = transport.send(envelope("msg_same"));
  await assert.rejects(transport.send(envelope("msg_same")), /message_id/);

  port.emitMessage(envelope("msg_same", "ack", { accepted: true }));
  assert.equal((await first).message_id, "msg_same");
});

test("helper connection performs fresh hello handshake over connectNative port", async () => {
  const port = new FakePort();
  port.onPost = (message) => queueMicrotask(() => port.emitMessage(helloAck(message)));
  const runtime = {
    connectCalls: [],
    connectNative(name) {
      this.connectCalls.push(name);
      return port;
    },
  };
  const connection = new HelperConnectionController({
    runtime,
    hostName: "uk.nasu.curiotrace",
    timeoutMs: 100,
  });

  const result = await connection.connect();
  assert.deepEqual(result, { accepted: true, reason: "OK" });
  assert.deepEqual(runtime.connectCalls, ["uk.nasu.curiotrace"]);
  assert.equal(port.sent.length, 1);
  assert.equal(port.sent[0].kind, "hello");
  assert.equal(connection.connected, true);
  assert.equal(connection.protocol.snapshot.handshakeComplete, true);
});

test("native port disconnect invalidates helper capture authority", async () => {
  const port = new FakePort();
  port.onPost = (message) =>
    queueMicrotask(() =>
      port.emitMessage(
        helloAck(message, {
          state: "RECORDING",
          sessionId: "ses_existing",
          epoch: 8,
        }),
      ),
    );
  const connection = new HelperConnectionController({
    runtime: { connectNative: () => port },
    hostName: "uk.nasu.curiotrace",
    timeoutMs: 100,
  });
  assert.equal((await connection.connect()).accepted, true);
  assert.equal(connection.protocol.authority.snapshot.captureAllowed, true);

  port.emitDisconnect();
  assert.equal(connection.connected, false);
  assert.equal(connection.protocol.authority.snapshot.captureAllowed, false);
  assert.equal(connection.protocol.authority.snapshot.sessionState, "INTERRUPTED");
});

test("incompatible handshake closes transport and remains fail-closed", async () => {
  const port = new FakePort();
  port.onPost = (message) => queueMicrotask(() => port.emitMessage(helloAck(message, { compatible: false })));
  const connection = new HelperConnectionController({
    runtime: { connectNative: () => port },
    hostName: "uk.nasu.curiotrace",
    timeoutMs: 100,
  });

  const result = await connection.connect();
  assert.equal(result.accepted, false);
  assert.equal(connection.connected, false);
  assert.equal(connection.protocol.authority.snapshot.captureAllowed, false);
  assert.equal(port.disconnectCalls, 1);
});

test("late disconnect from an old port cannot tear down a newer connection", async () => {
  const first = new FakePort();
  const second = new FakePort();
  for (const port of [first, second]) {
    port.onPost = (message) => queueMicrotask(() => port.emitMessage(helloAck(message)));
  }
  const ports = [first, second];
  const runtime = {
    connectNative() {
      return ports.shift();
    },
  };
  const connection = new HelperConnectionController({ runtime, hostName: "uk.nasu.curiotrace", timeoutMs: 100 });

  assert.equal((await connection.connect()).accepted, true);
  connection.disconnect();
  assert.equal(connection.connected, false);
  assert.equal((await connection.connect()).accepted, true);
  assert.equal(connection.connected, true);

  first.emitDisconnect();
  assert.equal(connection.connected, true);
  assert.equal(connection.protocol.snapshot.handshakeComplete, true);
});
