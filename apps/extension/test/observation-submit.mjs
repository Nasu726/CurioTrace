import assert from "node:assert/strict";
import test from "node:test";

import { CaptureAuthority } from "../dist/capture-authority.js";
import { ExtensionProtocolState } from "../dist/protocol-client.js";
import { ProtocolObservationPort } from "../dist/protocol-observation-submit.js";

function recordingProtocol() {
  const authority = new CaptureAuthority();
  const protocol = new ExtensionProtocolState({ authority });
  protocol.onTransportConnected();
  const hello = protocol.applyHelloAck({
    protocol_version: "1.0",
    message_id: "hello_1",
    kind: "hello.ack",
    payload: {
      compatible: true,
      session_state: "RECORDING",
      session_id: "ses_1",
      recording_epoch: 4,
    },
  });
  assert.equal(hello.accepted, true);
  return protocol;
}

function eventFor(token) {
  return {
    schema_version: "1.0",
    event_id: "evt_1",
    session_id: token.sessionId,
    recording_epoch: token.recordingEpoch,
    event_type: "visibility",
    wall_time: "2026-09-18T00:00:00.000Z",
    monotonic_ms: 10,
    payload: { fact: "tab_activated" },
  };
}

test("accepted observation keeps current recording authority", async () => {
  const protocol = recordingProtocol();
  const capture = protocol.authority.beginCapture();
  assert.equal(capture.allowed, true);
  let terminalFailures = 0;
  const port = new ProtocolObservationPort({
    protocol,
    transport: {
      async send(message) {
        return {
          protocol_version: "1.0",
          message_id: message.message_id,
          kind: "ack",
          payload: { accepted: true, reason: "OK" },
        };
      },
    },
    onTerminalFailure: () => { terminalFailures += 1; },
  });

  const result = await port.submit(eventFor(capture.token), capture.token);
  assert.deepEqual(result, { accepted: true, reason: "OK" });
  assert.equal(protocol.authority.snapshot.sessionState, "RECORDING");
  assert.equal(protocol.authority.snapshot.captureAllowed, true);
  assert.equal(terminalFailures, 0);
});

test("store failure ack applies helper Interrupted authority and terminates transport path", async () => {
  const protocol = recordingProtocol();
  const capture = protocol.authority.beginCapture();
  assert.equal(capture.allowed, true);
  const terminalReasons = [];
  const port = new ProtocolObservationPort({
    protocol,
    transport: {
      async send(message) {
        return {
          protocol_version: "1.0",
          message_id: message.message_id,
          kind: "ack",
          payload: {
            accepted: false,
            reason: "STORE_ERROR",
            state: "INTERRUPTED",
            session_id: "ses_1",
            recording_epoch: 5,
          },
        };
      },
    },
    onTerminalFailure: (reason) => terminalReasons.push(reason),
  });

  const result = await port.submit(eventFor(capture.token), capture.token);
  assert.deepEqual(result, { accepted: false, reason: "STORE_ERROR" });
  assert.equal(protocol.authority.snapshot.sessionState, "INTERRUPTED");
  assert.equal(protocol.authority.snapshot.recordingEpoch, 5);
  assert.equal(protocol.authority.snapshot.captureAllowed, false);
  assert.equal(protocol.authority.validateCaptureToken(capture.token).valid, false);
  assert.deepEqual(terminalReasons, ["STORE_ERROR"]);
});

test("authority rejection without helper state suspends local capture and terminates", async () => {
  const protocol = recordingProtocol();
  const capture = protocol.authority.beginCapture();
  assert.equal(capture.allowed, true);
  const terminalReasons = [];
  const port = new ProtocolObservationPort({
    protocol,
    transport: {
      async send(message) {
        return {
          protocol_version: "1.0",
          message_id: message.message_id,
          kind: "ack",
          payload: { accepted: false, reason: "STALE_EPOCH" },
        };
      },
    },
    onTerminalFailure: (reason) => terminalReasons.push(reason),
  });

  const result = await port.submit(eventFor(capture.token), capture.token);
  assert.deepEqual(result, { accepted: false, reason: "STALE_EPOCH" });
  assert.equal(protocol.authority.snapshot.sessionState, "RECORDING");
  assert.equal(protocol.authority.snapshot.captureAllowed, false);
  assert.deepEqual(terminalReasons, ["STALE_EPOCH"]);
});

test("schema or privacy rejection is terminal rather than silently dropping future observations", async () => {
  const protocol = recordingProtocol();
  const capture = protocol.authority.beginCapture();
  assert.equal(capture.allowed, true);
  const terminalReasons = [];
  const port = new ProtocolObservationPort({
    protocol,
    transport: {
      async send(message) {
        return {
          protocol_version: "1.0",
          message_id: message.message_id,
          kind: "ack",
          payload: { accepted: false, reason: "FORBIDDEN_PAYLOAD_FIELD:auth_token" },
        };
      },
    },
    onTerminalFailure: (reason) => terminalReasons.push(reason),
  });

  const result = await port.submit(eventFor(capture.token), capture.token);
  assert.deepEqual(result, { accepted: false, reason: "FORBIDDEN_PAYLOAD_FIELD:auth_token" });
  assert.equal(protocol.authority.snapshot.captureAllowed, false);
  assert.deepEqual(terminalReasons, ["FORBIDDEN_PAYLOAD_FIELD:auth_token"]);
});

test("malformed observation ack fails closed and invokes terminal cleanup", async () => {
  const protocol = recordingProtocol();
  const capture = protocol.authority.beginCapture();
  assert.equal(capture.allowed, true);
  const terminalReasons = [];
  const port = new ProtocolObservationPort({
    protocol,
    transport: {
      async send(message) {
        return {
          protocol_version: "1.0",
          message_id: message.message_id,
          kind: "not-an-ack",
          payload: {},
        };
      },
    },
    onTerminalFailure: (reason) => terminalReasons.push(reason),
  });

  const result = await port.submit(eventFor(capture.token), capture.token);
  assert.deepEqual(result, { accepted: false, reason: "MALFORMED_OBSERVATION_ACK" });
  assert.equal(protocol.authority.snapshot.captureAllowed, false);
  assert.deepEqual(terminalReasons, ["MALFORMED_OBSERVATION_ACK"]);
});
