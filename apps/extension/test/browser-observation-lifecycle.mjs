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
          session_state: "IDLE",
          session_id: null,
          recording_epoch: 0,
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
        session_id: state === "FINISHED" ? "ses_1" : "ses_1",
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

test("raw browser listeners exist only during helper-authorized Recording", async () => {
  const rawActivated = new FakeEvent();
  const rawUpdated = new FakeEvent();
  const rawRemoved = new FakeEvent();
  const rawFocus = new FakeEvent();
  const onMessage = new FakeEvent();
  const port = new FakePort();
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
      async contains() { return true; },
      async request() { return true; },
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

  const assertListenerCount = (count) => {
    assert.equal(rawActivated.listeners.length, count);
    assert.equal(rawUpdated.listeners.length, count);
    assert.equal(rawRemoved.listeners.length, count);
    assert.equal(rawFocus.listeners.length, count);
  };

  assert.equal(installed.browserEvents.active, false);
  assertListenerCount(0);

  const started = await sendBackground(onMessage, "curiotrace.session.start");
  assert.equal(started.accepted, true);
  assert.equal(started.state.authority.sessionState, "RECORDING");
  assert.equal(installed.browserEvents.active, true);
  assertListenerCount(1);

  const paused = await sendBackground(onMessage, "curiotrace.session.pause");
  assert.equal(paused.accepted, true);
  assert.equal(paused.state.authority.sessionState, "PAUSED");
  assert.equal(installed.browserEvents.active, false);
  assertListenerCount(0);

  const resumed = await sendBackground(onMessage, "curiotrace.session.resume");
  assert.equal(resumed.accepted, true);
  assert.equal(resumed.state.authority.sessionState, "RECORDING");
  assert.equal(installed.browserEvents.active, true);
  assertListenerCount(1);

  port.onDisconnect.emit();
  assert.equal(installed.browserEvents.active, false);
  assertListenerCount(0);
  assert.equal(installed.helper.protocol.authority.snapshot.captureAllowed, false);
});
