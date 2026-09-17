import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { BackgroundSessionBroker } from "../dist/background-broker.js";
import { installBackground, NATIVE_HOST_NAME } from "../dist/background.js";
import { HelperConnectionController } from "../dist/helper-connection.js";

class FakeEvent {
  listeners = [];

  addListener(listener) {
    this.listeners.push(listener);
  }

  emit(...args) {
    return this.listeners.map((listener) => listener(...args));
  }
}

class FakePort {
  onMessage = new FakeEvent();
  onDisconnect = new FakeEvent();
  onPost = null;
  sent = [];

  postMessage(message) {
    this.sent.push(message);
    this.onPost?.(message);
  }

  disconnect() {}

  emitMessage(message) {
    this.onMessage.emit(message);
  }
}

function helloAck(message) {
  return {
    protocol_version: "1.0",
    message_id: message.message_id,
    kind: "hello.ack",
    payload: {
      compatible: true,
      session_state: "IDLE",
      session_id: null,
      recording_epoch: 0,
    },
  };
}

function startAck(message) {
  return {
    protocol_version: "1.0",
    message_id: message.message_id,
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

function extensionSender(runtimeId = "ext_1") {
  return { id: runtimeId, url: "chrome-extension://abc/popup.html" };
}

test("background broker rejects content-script and foreign senders", async () => {
  const helper = new HelperConnectionController({
    runtime: { connectNative: () => { throw new Error("must not connect"); } },
    hostName: NATIVE_HOST_NAME,
  });
  const broker = new BackgroundSessionBroker({
    runtimeId: "ext_1",
    permissions: { contains: async () => true },
    helper,
  });

  const contentScript = await broker.handle(
    { kind: "curiotrace.session.start" },
    { id: "ext_1", url: "https://example.test/", tab: {} },
  );
  assert.deepEqual(contentScript, { accepted: false, reason: "UNTRUSTED_MESSAGE_SENDER" });

  const foreign = await broker.handle(
    { kind: "curiotrace.session.start" },
    { id: "other_extension", url: "chrome-extension://other/popup.html" },
  );
  assert.deepEqual(foreign, { accepted: false, reason: "UNTRUSTED_MESSAGE_SENDER" });
});

test("background broker rechecks host permission before connecting helper", async () => {
  let connects = 0;
  const helper = new HelperConnectionController({
    runtime: {
      connectNative() {
        connects += 1;
        throw new Error("unexpected connection");
      },
    },
    hostName: NATIVE_HOST_NAME,
  });
  const broker = new BackgroundSessionBroker({
    runtimeId: "ext_1",
    permissions: { contains: async () => false },
    helper,
  });

  const response = await broker.handle({ kind: "curiotrace.session.start" }, extensionSender());
  assert.deepEqual(response, { accepted: false, reason: "HOST_PERMISSION_REQUIRED" });
  assert.equal(connects, 0);
});

test("background broker connects, handshakes, and forwards one helper Start", async () => {
  const port = new FakePort();
  port.onPost = (message) =>
    queueMicrotask(() => {
      port.emitMessage(message.kind === "hello" ? helloAck(message) : startAck(message));
    });
  let connects = 0;
  const helper = new HelperConnectionController({
    runtime: {
      connectNative() {
        connects += 1;
        return port;
      },
    },
    hostName: NATIVE_HOST_NAME,
    timeoutMs: 100,
  });
  const broker = new BackgroundSessionBroker({
    runtimeId: "ext_1",
    permissions: { contains: async () => true },
    helper,
  });

  const response = await broker.handle({ kind: "curiotrace.session.start" }, extensionSender());
  assert.equal(response.accepted, true);
  assert.equal(response.reason, "OK");
  assert.equal(response.state.authority.sessionState, "RECORDING");
  assert.equal(response.state.authority.sessionId, "ses_1");
  assert.equal(response.state.authority.recordingEpoch, 1);
  assert.equal(response.state.authority.captureAllowed, true);
  assert.equal(connects, 1);
  assert.deepEqual(port.sent.map((message) => message.kind), ["hello", "session.start"]);
  assert.equal(helper.protocol.authority.snapshot.captureAllowed, true);
});

test("installBackground registers async own-extension message handler", async () => {
  const port = new FakePort();
  port.onPost = (message) => queueMicrotask(() => port.emitMessage(helloAck(message)));
  const onMessage = new FakeEvent();
  const runtime = {
    id: "ext_runtime",
    onMessage,
    connectNative(name) {
      assert.equal(name, NATIVE_HOST_NAME);
      return port;
    },
  };
  const permissions = {
    async contains() { return true; },
    async request() { return true; },
  };
  const installed = installBackground({ runtime, permissions });
  assert.equal(onMessage.listeners.length, 1);
  assert.equal(installed.helper.connected, false);

  const response = await new Promise((resolve) => {
    const keepOpen = onMessage.listeners[0](
      { kind: "curiotrace.helper.ensure-connected" },
      { id: "ext_runtime", url: "moz-extension://runtime/popup.html" },
      resolve,
    );
    assert.equal(keepOpen, true);
  });
  assert.deepEqual(response, { accepted: true, reason: "OK" });
  assert.equal(installed.helper.connected, true);
});

test("manifest keeps broad hosts optional and supports Chrome/Firefox MV3 backgrounds", async () => {
  const manifest = JSON.parse(await readFile(new URL("../manifest.json", import.meta.url), "utf8"));
  assert.equal(manifest.manifest_version, 3);
  assert.deepEqual(manifest.permissions, ["nativeMessaging"]);
  assert.deepEqual(manifest.optional_host_permissions, ["http://*/*", "https://*/*"]);
  assert.equal(manifest.background.service_worker, "dist/background.js");
  assert.deepEqual(manifest.background.scripts, ["dist/background.js"]);
  assert.equal(manifest.background.type, "module");
  assert.equal(manifest.browser_specific_settings.gecko.id, "curiotrace@nasu.uk");
  assert.equal("host_permissions" in manifest, false);
});
