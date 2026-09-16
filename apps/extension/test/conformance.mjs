import assert from "node:assert/strict";
import test from "node:test";

import { CaptureAuthority } from "../dist/capture-authority.js";
import { ExtensionProtocolState } from "../dist/protocol-client.js";

function helloAck({ state = "IDLE", sessionId = null, epoch = 0 } = {}) {
  return {
    protocol_version: "1.0",
    message_id: "hello",
    kind: "hello.ack",
    payload: {
      compatible: true,
      session_state: state,
      session_id: sessionId,
      recording_epoch: epoch,
    },
  };
}

function controlAck({ state, sessionId, epoch }) {
  return {
    protocol_version: "1.0",
    message_id: "ack",
    kind: "ack",
    payload: {
      accepted: true,
      reason: "OK",
      state,
      session_id: sessionId,
      recording_epoch: epoch,
    },
  };
}

test("capture authority fails closed until helper state grants recording", () => {
  const authority = new CaptureAuthority();
  assert.equal(authority.beginCapture().allowed, false);
  authority.connect();
  assert.equal(authority.beginCapture().allowed, false);

  const applied = authority.applyHelperState({ state: "RECORDING", sessionId: "ses_1", recordingEpoch: 1 });
  assert.equal(applied.accepted, true);
  assert.equal(authority.beginCapture().allowed, true);
});

test("local Pause suspension invalidates in-flight work before helper ack", () => {
  const state = new ExtensionProtocolState();
  state.onTransportConnected();
  assert.equal(state.applyHelloAck(helloAck()).accepted, true);

  const start = state.buildStartRequest();
  assert.equal(start.allowed, true);
  assert.equal(state.applyControlAck("session.start", controlAck({ state: "RECORDING", sessionId: "ses_1", epoch: 1 })).accepted, true);

  const token = state.authority.beginCapture().token;
  assert.ok(token);
  const pause = state.buildPauseRequest();
  assert.equal(pause.allowed, true);
  assert.equal(state.authority.snapshot.captureAllowed, false);
  assert.equal(state.authority.validateCaptureToken(token).valid, false);
});

test("reconnection never restores old capture token", () => {
  const state = new ExtensionProtocolState();
  state.onTransportConnected();
  state.applyHelloAck(helloAck({ state: "RECORDING", sessionId: "ses_same", epoch: 7 }));
  const token = state.authority.beginCapture().token;
  assert.ok(token);

  state.onTransportDisconnected();
  state.onTransportConnected();
  state.applyHelloAck(helloAck({ state: "RECORDING", sessionId: "ses_same", epoch: 7 }));

  const validation = state.authority.validateCaptureToken(token);
  assert.equal(validation.valid, false);
  assert.equal(validation.reason, "STALE_CONNECTION");
});

test("observation submission requires event and capture-token authority to match", () => {
  const state = new ExtensionProtocolState();
  state.onTransportConnected();
  state.applyHelloAck(helloAck({ state: "RECORDING", sessionId: "ses_1", epoch: 9 }));
  const token = state.authority.beginCapture().token;
  assert.ok(token);

  const bad = state.buildObservationSubmit({ session_id: "ses_1", recording_epoch: 8, event_type: "content_observation" }, token);
  assert.equal(bad.allowed, false);
  assert.equal(bad.reason, "EVENT_AUTHORITY_MISMATCH");

  const good = state.buildObservationSubmit({ session_id: "ses_1", recording_epoch: 9, event_type: "content_observation" }, token);
  assert.equal(good.allowed, true);
  assert.equal(good.message.kind, "observation.submit");
});
