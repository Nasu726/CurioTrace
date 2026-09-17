import assert from "node:assert/strict";
import test from "node:test";

import { installBackground } from "../dist/background.js";

class FakeEvent {
  listeners = [];
  addListener(listener) {
    if (!this.listeners.includes(listener)) this.listeners.push(listener);
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
  epoch = 0;
  initialState;

  constructor({ initialState = "IDLE", initialEpoch = 0 } = {}) {
    this.initialState = initialState;
    this.epoch = initialEpoch;
  }

  postMessage(message) {
    this.sent.push(message);
    queueMicrotask(() => this.#reply(message));
  }

  disconnect() {
    this.onDisconnect.emit();
  }

  #reply(message) {
    if (message.kind === "hello") {
      this.onMessage.emit({
        protocol_version: "1.0",
        message_id: message.message_id,
        kind: "hello.ack",
        payload: {
          compatible: true,
          session_state: this.initialState,
          session_id: this.initialState === "IDLE" ? null : "ses_1",
          recording_epoch: this.epoch,
        },
      });
      return;
    }
    if (message.kind === "session.start") {
      this.epoch = 1;
      this.#controlAck(message, "RECORDING");
      return;
    }
    if (message.kind === "session.pause") {
      this.epoch += 1;
      this.#controlAck(message, "PAUSED");
      return;
    }
    if (message.kind === "session.resume") {
      this.epoch += 1;
      this.#controlAck(message, "RECORDING");
      return;
    }
    if (message.kind === "session.stop") {
      this.epoch += 1;
      this.#controlAck(message, "FINISHED");
      return;
    }
    if (message.kind === "observation.submit") {
      this.onMessage.emit({
        protocol_version: "1.0",
        message_id: message.message_id,
        kind: "ack",
        payload: { accepted: true, reason: "OK" },
      });
      return;
    }
    throw new Error(`unexpected message ${message.kind}`);
  }

  #controlAck(message, state) {
    this.onMessage.emit({
      protocol_version: "1.0",
      message_id: message.message_id,
      kind: "ack",
      payload: {
        accepted: true,
        reason: "OK",
        state,
        session_id: "ses_1",
        recording_epoch: this.epoch,
      },
    });
  }
}

function sendBackground(onMessage, kind) {
  return new Promise((resolve) => {
    const keepOpen = onMessage.listeners[0](
      { kind },
      { id: "ext_runtime", url: "chrome-extension://runtime/popup.html" },
      resolve,
    );
    assert.equal(keepOpen, true);
  });
}

function harness({ port = new FakePort(), permissionGranted = true } = {}) {
  const rawActivated = new FakeEvent();
  const rawUpdated = new FakeEvent();
  const rawRemoved = new FakeEvent();
  const rawFocus = new FakeEvent();
  const onMessage = new FakeEvent();
  const onPermissionRemoved = new FakeEvent();
  const permissionState = { granted: permissionGranted };
  const activeTab = {
    id: 10,
    windowId: 20,
    active: true,
    incognito: false,
    url: "https://example.test/article",
    title: "Article",
  };

  const installed = installBackground({
    runtime: {
      id: "ext_runtime",
      onMessage,
      connectNative() { return port; },
    },
    permissions: {
      async contains() { return permissionState.granted; },
      async request() { return permissionState.granted; },
      onRemoved: onPermissionRemoved,
    },
    tabs: {
      onActivated: rawActivated,
      onUpdated: rawUpdated,
      onRemoved: rawRemoved,
      async get() { return activeTab; },
      async query() { return [activeTab]; },
    },
    windows: {
      WINDOW_ID_NONE: -1,
      onFocusChanged: rawFocus,
    },
  });

  return {
    installed,
    onMessage,
    onPermissionRemoved,
    permissionState,
    port,
    rawEvents: [rawActivated, rawUpdated, rawRemoved, rawFocus],
  };
}

function assertListenerCount(rawEvents, count) {
  for (const event of rawEvents) {
    assert.equal(event.listeners.length, count);
  }
}

test("raw browser listeners exist only during helper-authorized Recording", async () => {
  const h = harness();

  assert.equal(h.installed.browserEvents.active, false);
  assertListenerCount(h.rawEvents, 0);

  const started = await sendBackground(h.onMessage, "curiotrace.session.start");
  assert.equal(started.accepted, true);
  assert.equal(started.state.authority.sessionState, "RECORDING");
  assert.equal(h.installed.browserEvents.active, true);
  assertListenerCount(h.rawEvents, 1);

  const paused = await sendBackground(h.onMessage, "curiotrace.session.pause");
  assert.equal(paused.accepted, true);
  assert.equal(paused.state.authority.sessionState, "PAUSED");
  assert.equal(h.installed.browserEvents.active, false);
  assertListenerCount(h.rawEvents, 0);

  h.permissionState.granted = false;
  const deniedResume = await sendBackground(h.onMessage, "curiotrace.session.resume");
  assert.equal(deniedResume.accepted, false);
  assert.equal(deniedResume.reason, "HOST_PERMISSION_REQUIRED");
  assert.equal(h.installed.browserEvents.active, false);
  assertListenerCount(h.rawEvents, 0);
  assert.equal(h.port.sent.some((message) => message.kind === "session.resume"), false);

  h.permissionState.granted = true;
  const resumed = await sendBackground(h.onMessage, "curiotrace.session.resume");
  assert.equal(resumed.accepted, true);
  assert.equal(resumed.state.authority.sessionState, "RECORDING");
  assert.equal(h.installed.browserEvents.active, true);
  assertListenerCount(h.rawEvents, 1);

  h.permissionState.granted = false;
  h.onPermissionRemoved.emit({ origins: ["https://*/*"] });
  assert.equal(h.installed.browserEvents.active, false);
  assertListenerCount(h.rawEvents, 0);
  assert.equal(h.installed.helper.connected, false);
  assert.equal(h.installed.helper.protocol.authority.snapshot.captureAllowed, false);
});

test("fresh handshake that reports Recording restores observation only after host permission check", async () => {
  const h = harness({ port: new FakePort({ initialState: "RECORDING", initialEpoch: 7 }) });
  assertListenerCount(h.rawEvents, 0);

  const state = await sendBackground(h.onMessage, "curiotrace.state.get");
  assert.equal(state.accepted, true);
  assert.equal(state.state.authority.sessionState, "RECORDING");
  assert.equal(state.state.authority.recordingEpoch, 7);
  assert.equal(h.installed.browserEvents.active, true);
  assertListenerCount(h.rawEvents, 1);
  assert.equal(h.port.sent.filter((message) => message.kind === "observation.submit").length, 2);
});

test("recovered Recording fails closed when required host permission is absent", async () => {
  const h = harness({
    port: new FakePort({ initialState: "RECORDING", initialEpoch: 7 }),
    permissionGranted: false,
  });

  const state = await sendBackground(h.onMessage, "curiotrace.state.get");
  assert.equal(state.accepted, false);
  assert.equal(state.reason, "HOST_PERMISSION_REQUIRED");
  assert.equal(h.installed.browserEvents.active, false);
  assertListenerCount(h.rawEvents, 0);
  assert.equal(h.installed.helper.connected, false);
});
