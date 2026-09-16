import assert from "node:assert/strict";
import test from "node:test";

import { PermissionStartController } from "../dist/permission-controller.js";
import { ExtensionProtocolState } from "../dist/protocol-client.js";
import { ProtocolSessionStartPort } from "../dist/protocol-session-start.js";

function helloAck() {
  return {
    protocol_version: "1.0",
    message_id: "hello",
    kind: "hello.ack",
    payload: {
      compatible: true,
      session_state: "IDLE",
      session_id: null,
      recording_epoch: 0,
    },
  };
}

function recordingAck() {
  return {
    protocol_version: "1.0",
    message_id: "start",
    kind: "ack",
    payload: {
      accepted: true,
      reason: "OK",
      state: "RECORDING",
      session_id: "ses_1",
      recording_epoch: 1,
    },
  };
}

function readyProtocol() {
  const protocol = new ExtensionProtocolState();
  protocol.onTransportConnected();
  assert.equal(protocol.applyHelloAck(helloAck()).accepted, true);
  return protocol;
}

test("protocol Start adapter sends exactly one helper start and applies authority", async () => {
  const protocol = readyProtocol();
  const seen = [];
  const adapter = new ProtocolSessionStartPort({
    protocol,
    transport: {
      async send(message) {
        seen.push(message);
        return recordingAck();
      },
    },
  });

  const result = await adapter.startSession();
  assert.deepEqual(result, { accepted: true, reason: "OK" });
  assert.equal(seen.length, 1);
  assert.equal(seen[0].kind, "session.start");
  assert.equal(protocol.authority.snapshot.sessionState, "RECORDING");
  assert.equal(protocol.authority.snapshot.captureAllowed, true);
});

test("protocol Start adapter never sends before handshake", async () => {
  const protocol = new ExtensionProtocolState();
  let sends = 0;
  const adapter = new ProtocolSessionStartPort({
    protocol,
    transport: {
      async send() {
        sends += 1;
        return recordingAck();
      },
    },
  });

  const result = await adapter.startSession();
  assert.deepEqual(result, { accepted: false, reason: "HANDSHAKE_REQUIRED" });
  assert.equal(sends, 0);
});

test("transport failure never grants recording authority", async () => {
  const protocol = readyProtocol();
  const adapter = new ProtocolSessionStartPort({
    protocol,
    transport: {
      async send() {
        throw new Error("native port failed");
      },
    },
  });

  const result = await adapter.startSession();
  assert.deepEqual(result, { accepted: false, reason: "TRANSPORT_ERROR" });
  assert.equal(protocol.authority.snapshot.captureAllowed, false);
});

test("permission denial prevents helper session.start from crossing transport", async () => {
  const protocol = readyProtocol();
  let sends = 0;
  const session = new ProtocolSessionStartPort({
    protocol,
    transport: {
      async send() {
        sends += 1;
        return recordingAck();
      },
    },
  });
  const permissions = {
    async contains() {
      return false;
    },
    async request() {
      return false;
    },
  };
  const flow = new PermissionStartController({ permissions, session });

  assert.equal((await flow.beginStart()).kind, "EXPLANATION_REQUIRED");
  const denied = await flow.continueAfterExplanation();
  assert.deepEqual(denied, { kind: "NOT_STARTED", reason: "PERMISSION_DENIED" });
  assert.equal(sends, 0);
  assert.equal(protocol.authority.snapshot.sessionState, "IDLE");
});

test("permission grant can advance through helper authority to recording", async () => {
  const protocol = readyProtocol();
  let sends = 0;
  let granted = false;
  const session = new ProtocolSessionStartPort({
    protocol,
    transport: {
      async send(message) {
        sends += 1;
        assert.equal(message.kind, "session.start");
        return recordingAck();
      },
    },
  });
  const permissions = {
    async contains() {
      return granted;
    },
    async request() {
      granted = true;
      return true;
    },
  };
  const flow = new PermissionStartController({ permissions, session });

  assert.equal((await flow.beginStart()).kind, "EXPLANATION_REQUIRED");
  const started = await flow.continueAfterExplanation();
  assert.deepEqual(started, { kind: "STARTED" });
  assert.equal(sends, 1);
  assert.equal(protocol.authority.snapshot.sessionState, "RECORDING");
  assert.equal(protocol.authority.snapshot.captureAllowed, true);
});
