import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { ExtensionProtocolState } from "../dist/protocol-client.js";
import { ProtocolSessionControlPort } from "../dist/protocol-session-control.js";
import { ProtocolSessionStartPort } from "../dist/protocol-session-start.js";
import { popupViewForState } from "../dist/popup-view-model.js";
import { RuntimeSessionControlPort } from "../dist/runtime-session-control.js";

function readyProtocol() {
  const protocol = new ExtensionProtocolState();
  protocol.onTransportConnected();
  assert.equal(
    protocol.applyHelloAck({
      protocol_version: "1.0",
      message_id: "hello",
      kind: "hello.ack",
      payload: {
        compatible: true,
        session_state: "IDLE",
        session_id: null,
        recording_epoch: 0,
      },
    }).accepted,
    true,
  );
  return protocol;
}

function ackFor(message, state, epoch) {
  return {
    protocol_version: "1.0",
    message_id: message.message_id,
    kind: "ack",
    payload: {
      accepted: true,
      reason: "OK",
      state,
      session_id: "ses_1",
      recording_epoch: epoch,
    },
  };
}

test("Pause Resume Stop use helper acknowledgements and advance local authority", async () => {
  const protocol = readyProtocol();
  const seen = [];
  let epoch = 0;
  const transport = {
    async send(message) {
      seen.push(message.kind);
      epoch += 1;
      const state =
        message.kind === "session.start" || message.kind === "session.resume"
          ? "RECORDING"
          : message.kind === "session.pause"
            ? "PAUSED"
            : "FINISHED";
      return ackFor(message, state, epoch);
    },
  };

  const start = new ProtocolSessionStartPort({ protocol, transport });
  const controls = new ProtocolSessionControlPort({ protocol, transport });
  assert.deepEqual(await start.startSession(), { accepted: true, reason: "OK" });
  assert.equal(protocol.authority.snapshot.sessionState, "RECORDING");

  assert.deepEqual(await controls.pause(), { accepted: true, reason: "OK" });
  assert.equal(protocol.authority.snapshot.sessionState, "PAUSED");
  assert.equal(protocol.authority.snapshot.captureAllowed, false);

  assert.deepEqual(await controls.resume(), { accepted: true, reason: "OK" });
  assert.equal(protocol.authority.snapshot.sessionState, "RECORDING");
  assert.equal(protocol.authority.snapshot.captureAllowed, true);

  assert.deepEqual(await controls.stop(), { accepted: true, reason: "OK" });
  assert.equal(protocol.authority.snapshot.sessionState, "FINISHED");
  assert.equal(protocol.authority.snapshot.captureAllowed, false);
  assert.deepEqual(seen, ["session.start", "session.pause", "session.resume", "session.stop"]);
});

test("control transport failure leaves local capture suspended", async () => {
  const protocol = readyProtocol();
  const start = new ProtocolSessionStartPort({
    protocol,
    transport: { send: async (message) => ackFor(message, "RECORDING", 1) },
  });
  assert.equal((await start.startSession()).accepted, true);
  assert.equal(protocol.authority.snapshot.captureAllowed, true);

  const controls = new ProtocolSessionControlPort({
    protocol,
    transport: { send: async () => { throw new Error("port lost"); } },
  });
  const paused = await controls.pause();
  assert.deepEqual(paused, { accepted: false, reason: "TRANSPORT_ERROR" });
  assert.equal(protocol.authority.snapshot.captureAllowed, false);
});

test("runtime control bridge validates state and lifecycle responses", async () => {
  const sent = [];
  const runtime = {
    async sendMessage(message) {
      sent.push(message.kind);
      if (message.kind === "curiotrace.state.get") {
        return {
          accepted: true,
          reason: "OK",
          state: {
            handshakeComplete: true,
            authority: {
              helperConnected: true,
              sessionState: "PAUSED",
              sessionId: "ses_1",
              recordingEpoch: 4,
            },
          },
        };
      }
      return { accepted: true, reason: "OK" };
    },
  };
  const controls = new RuntimeSessionControlPort(runtime);
  assert.deepEqual(await controls.pause(), { accepted: true, reason: "OK" });
  assert.deepEqual(await controls.resume(), { accepted: true, reason: "OK" });
  assert.deepEqual(await controls.stop(), { accepted: true, reason: "OK" });
  assert.deepEqual(await controls.state(), {
    accepted: true,
    reason: "OK",
    sessionState: "PAUSED",
    sessionId: "ses_1",
    recordingEpoch: 4,
    helperConnected: true,
  });
  assert.deepEqual(sent, [
    "curiotrace.session.pause",
    "curiotrace.session.resume",
    "curiotrace.session.stop",
    "curiotrace.state.get",
  ]);
});

test("popup view exposes only lifecycle-valid actions", () => {
  assert.deepEqual(popupViewForState("IDLE").actions, ["start"]);
  assert.deepEqual(popupViewForState("RECORDING").actions, ["pause", "stop"]);
  assert.deepEqual(popupViewForState("PAUSED").actions, ["resume", "stop"]);
  assert.deepEqual(popupViewForState("INTERRUPTED").actions, ["resume", "stop"]);
  assert.deepEqual(popupViewForState("FINISHED").actions, ["start"]);
  assert.equal(popupViewForState("RECORDING").recording, true);
  assert.equal(popupViewForState("PAUSED").recording, false);
});

test("manifest and popup package reference built popup module", async () => {
  const manifest = JSON.parse(await readFile(new URL("../manifest.json", import.meta.url), "utf8"));
  const html = await readFile(new URL("../popup.html", import.meta.url), "utf8");
  assert.equal(manifest.action.default_popup, "popup.html");
  assert.match(html, /dist\/popup\.js/);
  assert.match(html, /id="continue-button"/);
  assert.match(html, /id="cancel-button"/);
});
