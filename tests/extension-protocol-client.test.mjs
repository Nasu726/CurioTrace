import assert from "node:assert/strict";
import test from "node:test";

import { ExtensionProtocolState } from "../spikes/extension_authority_harness/protocol-client-state.mjs";

function helloAck({ state = "IDLE", sessionId = null, epoch = 0 } = {}) {
  return {
    protocol_version: "1.0",
    kind: "hello.ack",
    payload: {
      compatible: true,
      session_state: state,
      session_id: sessionId,
      recording_epoch: epoch,
    },
  };
}

function ack({ state, sessionId, epoch, accepted = true, reason = "OK" }) {
  return {
    protocol_version: "1.0",
    kind: "ack",
    payload: {
      accepted,
      reason,
      state,
      session_id: sessionId,
      recording_epoch: epoch,
    },
  };
}

function enterRecording(client, { sessionId = "ses_test", epoch = 1 } = {}) {
  client.onTransportConnected();
  assert.equal(client.applyHelloAck(helloAck()).accepted, true);
  const start = client.buildStartRequest();
  assert.equal(start.allowed, true);
  assert.equal(
    client.applyControlAck(
      "session.start",
      ack({ state: "RECORDING", sessionId, epoch })
    ).accepted,
    true
  );
}

test("new connection requires handshake before Start", () => {
  const client = new ExtensionProtocolState();
  const hello = client.onTransportConnected();
  assert.equal(hello.kind, "hello");
  assert.equal(client.buildStartRequest().allowed, false);

  assert.equal(client.applyHelloAck(helloAck()).accepted, true);
  assert.equal(client.buildStartRequest().allowed, true);
});

test("incompatible handshake never grants capture", () => {
  const client = new ExtensionProtocolState();
  client.onTransportConnected();

  const result = client.applyHelloAck({
    protocol_version: "2.0",
    kind: "hello.ack",
    payload: { compatible: true, session_state: "RECORDING", session_id: "ses", recording_epoch: 1 },
  });

  assert.equal(result.accepted, false);
  assert.equal(client.snapshot.handshakeComplete, false);
  assert.equal(client.snapshot.authority.captureAllowed, false);
});

test("Pause request disables local capture before acknowledgement", () => {
  const client = new ExtensionProtocolState();
  enterRecording(client, { epoch: 4 });
  const inFlight = client.authority.beginCapture().token;

  const pause = client.buildPauseRequest();
  assert.equal(pause.allowed, true);
  assert.equal(client.snapshot.authority.captureAllowed, false);
  assert.equal(client.authority.validateCaptureToken(inFlight).valid, false);

  assert.equal(
    client.applyControlAck(
      "session.pause",
      ack({ state: "PAUSED", sessionId: "ses_test", epoch: 5 })
    ).accepted,
    true
  );
  assert.equal(client.snapshot.authority.sessionState, "PAUSED");
});

test("failed Pause acknowledgement does not optimistically resume capture", () => {
  const client = new ExtensionProtocolState();
  enterRecording(client, { epoch: 6 });

  client.buildPauseRequest();
  const result = client.applyControlAck(
    "session.pause",
    ack({ state: "RECORDING", sessionId: "ses_test", epoch: 6, accepted: false, reason: "CONTROL_REJECTED" })
  );

  assert.equal(result.accepted, false);
  assert.equal(client.snapshot.authority.captureAllowed, false);
});

test("Resume acknowledgement with new epoch restores capture", () => {
  const client = new ExtensionProtocolState();
  enterRecording(client, { epoch: 10 });
  client.buildPauseRequest();
  client.applyControlAck(
    "session.pause",
    ack({ state: "PAUSED", sessionId: "ses_test", epoch: 11 })
  );

  const resume = client.buildResumeRequest();
  assert.equal(resume.allowed, true);
  assert.equal(
    client.applyControlAck(
      "session.resume",
      ack({ state: "RECORDING", sessionId: "ses_test", epoch: 12 })
    ).accepted,
    true
  );
  assert.equal(client.snapshot.authority.captureAllowed, true);
  assert.equal(client.snapshot.authority.recordingEpoch, 12);
});

test("transport disconnect invalidates handshake and capture authority", () => {
  const client = new ExtensionProtocolState();
  enterRecording(client, { epoch: 15 });

  client.onTransportDisconnected();

  assert.equal(client.snapshot.handshakeComplete, false);
  assert.equal(client.snapshot.authority.captureAllowed, false);
  assert.equal(client.buildPauseRequest().allowed, false);
});

test("observation submit requires the original current capture token", () => {
  const client = new ExtensionProtocolState();
  enterRecording(client, { sessionId: "ses_obs", epoch: 30 });
  const token = client.authority.beginCapture().token;
  const event = {
    schema_version: "1.0",
    event_id: "evt_1",
    session_id: "ses_obs",
    recording_epoch: 30,
    event_type: "content_observation",
    wall_time: "2026-09-16T09:30:00Z",
    monotonic_ms: 1,
    capture_mode: "dom",
    payload: { units: [] },
  };

  assert.equal(client.buildObservationSubmit(event, token).allowed, true);

  client.onTransportDisconnected();
  client.onTransportConnected();
  client.applyHelloAck(helloAck({ state: "RECORDING", sessionId: "ses_obs", epoch: 30 }));

  const stale = client.buildObservationSubmit(event, token);
  assert.equal(stale.allowed, false);
  assert.equal(stale.reason, "STALE_CONNECTION");
});

test("event authority must match capture token", () => {
  const client = new ExtensionProtocolState();
  enterRecording(client, { sessionId: "ses_a", epoch: 40 });
  const token = client.authority.beginCapture().token;

  const result = client.buildObservationSubmit(
    {
      session_id: "ses_other",
      recording_epoch: 40,
      event_type: "content_observation",
      payload: {},
    },
    token
  );

  assert.equal(result.allowed, false);
  assert.equal(result.reason, "EVENT_AUTHORITY_MISMATCH");
});
