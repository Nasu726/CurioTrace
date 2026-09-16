import assert from "node:assert/strict";
import test from "node:test";

import { CaptureAuthority } from "../spikes/extension_authority_harness/capture-authority.mjs";

function recording(authority, { sessionId = "ses_test", epoch = 1 } = {}) {
  authority.connect();
  const applied = authority.applyHelperState({
    state: "RECORDING",
    sessionId,
    recordingEpoch: epoch,
  });
  assert.equal(applied.accepted, true);
}

test("capture is denied before helper recording authority exists", () => {
  const authority = new CaptureAuthority();
  assert.equal(authority.beginCapture().allowed, false);

  authority.connect();
  assert.equal(authority.beginCapture().allowed, false);
});

test("current recording authority allows capture", () => {
  const authority = new CaptureAuthority();
  recording(authority, { epoch: 7 });

  const started = authority.beginCapture();
  assert.equal(started.allowed, true);
  assert.equal(started.token.recordingEpoch, 7);
});

test("Pause invalidates an in-flight capture token", () => {
  const authority = new CaptureAuthority();
  recording(authority, { epoch: 10 });
  const token = authority.beginCapture().token;

  authority.applyHelperState({
    state: "PAUSED",
    sessionId: "ses_test",
    recordingEpoch: 11,
  });

  let builderCalled = false;
  const result = authority.finalizeCapture(token, () => {
    builderCalled = true;
    return { event_type: "content_observation" };
  });

  assert.equal(result.accepted, false);
  assert.equal(builderCalled, false);
  assert.equal(result.observation, null);
});

test("Resume uses a new epoch and old async results stay invalid", () => {
  const authority = new CaptureAuthority();
  recording(authority, { epoch: 20 });
  const oldToken = authority.beginCapture().token;

  authority.applyHelperState({ state: "PAUSED", sessionId: "ses_test", recordingEpoch: 21 });
  authority.applyHelperState({ state: "RECORDING", sessionId: "ses_test", recordingEpoch: 22 });

  assert.equal(authority.validateCaptureToken(oldToken).valid, false);

  const newToken = authority.beginCapture().token;
  const finalized = authority.finalizeCapture(newToken, () => ({
    schema_version: "1.0",
    event_type: "content_observation",
    payload: { units: [] },
  }));

  assert.equal(finalized.accepted, true);
  assert.equal(finalized.observation.session_id, "ses_test");
  assert.equal(finalized.observation.recording_epoch, 22);
});

test("helper disconnect immediately removes capture authority", () => {
  const authority = new CaptureAuthority();
  recording(authority, { epoch: 2 });
  const token = authority.beginCapture().token;

  authority.disconnect();

  assert.equal(authority.snapshot.captureAllowed, false);
  assert.equal(authority.snapshot.sessionState, "INTERRUPTED");
  assert.equal(authority.validateCaptureToken(token).valid, false);
  assert.equal(authority.beginCapture().allowed, false);
});

test("reconnect never restores old recording authority implicitly", () => {
  const authority = new CaptureAuthority();
  recording(authority, { epoch: 3 });
  const token = authority.beginCapture().token;

  authority.disconnect();
  authority.connect();

  assert.equal(authority.snapshot.sessionState, "IDLE");
  assert.equal(authority.snapshot.captureAllowed, false);
  assert.equal(authority.validateCaptureToken(token).valid, false);
});

test("transport reconnection invalidates token even if session and epoch values repeat", () => {
  const authority = new CaptureAuthority();
  recording(authority, { sessionId: "ses_same", epoch: 50 });
  const oldToken = authority.beginCapture().token;

  authority.disconnect();
  authority.connect();
  authority.applyHelperState({
    state: "RECORDING",
    sessionId: "ses_same",
    recordingEpoch: 50,
  });

  const validation = authority.validateCaptureToken(oldToken);
  assert.equal(validation.valid, false);
  assert.equal(validation.reason, "STALE_CONNECTION");
});

test("malformed RECORDING helper state is not accepted", () => {
  const authority = new CaptureAuthority();
  authority.connect();

  const result = authority.applyHelperState({ state: "RECORDING" });
  assert.equal(result.accepted, false);
  assert.equal(result.reason, "INCOMPLETE_RECORDING_AUTHORITY");
  assert.equal(authority.captureAllowed, false);
});

test("finalizeCapture never builds an observation after Stop", () => {
  const authority = new CaptureAuthority();
  recording(authority, { epoch: 100 });
  const token = authority.beginCapture().token;

  authority.applyHelperState({
    state: "FINISHED",
    sessionId: "ses_test",
    recordingEpoch: 101,
  });

  let called = false;
  const result = authority.finalizeCapture(token, () => {
    called = true;
    return { forbidden: "should never be materialized" };
  });

  assert.equal(result.accepted, false);
  assert.equal(called, false);
});
