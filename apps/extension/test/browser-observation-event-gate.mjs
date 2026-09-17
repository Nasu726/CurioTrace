import assert from "node:assert/strict";
import test from "node:test";

import { BrowserObservationEventGate } from "../dist/browser-observation-event-gate.js";

class FakeRawEvent {
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

function rawApis() {
  const onActivated = new FakeRawEvent();
  const onUpdated = new FakeRawEvent();
  const onRemoved = new FakeRawEvent();
  const onFocusChanged = new FakeRawEvent();
  return {
    tabs: {
      onActivated,
      onUpdated,
      onRemoved,
      async get() { return {}; },
      async query() { return []; },
    },
    windows: { WINDOW_ID_NONE: -1, onFocusChanged },
    events: { onActivated, onUpdated, onRemoved, onFocusChanged },
  };
}

test("browser observation listeners are not attached until the recording gate activates", () => {
  const raw = rawApis();
  const gate = new BrowserObservationEventGate(raw.tabs, raw.windows);
  let updates = 0;
  gate.tabs.onUpdated.addListener(() => { updates += 1; });

  assert.equal(gate.active, false);
  assert.equal(raw.events.onUpdated.listeners.length, 0);
  raw.events.onUpdated.emit(1, { url: "https://idle.example/" }, {});
  assert.equal(updates, 0);

  gate.activate();
  assert.equal(gate.active, true);
  assert.equal(raw.events.onUpdated.listeners.length, 1);
  raw.events.onUpdated.emit(1, { url: "https://recording.example/" }, {});
  assert.equal(updates, 1);

  gate.deactivate();
  assert.equal(gate.active, false);
  assert.equal(raw.events.onUpdated.listeners.length, 0);
  raw.events.onUpdated.emit(1, { url: "https://paused.example/" }, {});
  assert.equal(updates, 1);
});

test("activate and deactivate are idempotent across all browser observation events", () => {
  const raw = rawApis();
  const gate = new BrowserObservationEventGate(raw.tabs, raw.windows);
  gate.tabs.onActivated.addListener(() => {});
  gate.tabs.onUpdated.addListener(() => {});
  gate.tabs.onRemoved.addListener(() => {});
  gate.windows.onFocusChanged.addListener(() => {});

  gate.activate();
  gate.activate();
  for (const event of Object.values(raw.events)) assert.equal(event.listeners.length, 1);

  gate.deactivate();
  gate.deactivate();
  for (const event of Object.values(raw.events)) assert.equal(event.listeners.length, 0);
});
