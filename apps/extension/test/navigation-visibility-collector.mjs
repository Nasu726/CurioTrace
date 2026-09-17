import assert from "node:assert/strict";
import test from "node:test";

import { CaptureAuthority } from "../dist/capture-authority.js";
import { NavigationVisibilityCollector } from "../dist/navigation-visibility-collector.js";
import { ObservationEventFactory } from "../dist/observation-event.js";

class FakeEvent {
  listeners = [];
  addListener(listener) {
    this.listeners.push(listener);
  }
  emit(...args) {
    for (const listener of this.listeners) {
      listener(...args);
    }
  }
}

function recordingAuthority() {
  const authority = new CaptureAuthority();
  authority.connect();
  const applied = authority.applyHelperState({
    state: "RECORDING",
    sessionId: "ses_1",
    recordingEpoch: 7,
  });
  assert.equal(applied.accepted, true);
  return authority;
}

function deterministicEvents() {
  let sequence = 0;
  return new ObservationEventFactory({
    clock: {
      wallTime: () => "2026-09-18T00:00:00.000Z",
      monotonicMs: () => 42.5,
    },
    ids: {
      next(prefix) {
        sequence += 1;
        return `${prefix}_${sequence}`;
      },
    },
  });
}

function harness({ authority = recordingAuthority(), activeTab } = {}) {
  const onActivated = new FakeEvent();
  const onUpdated = new FakeEvent();
  const onRemoved = new FakeEvent();
  const onFocusChanged = new FakeEvent();
  const events = [];
  let getCalls = 0;
  let queryCalls = 0;
  const tabsById = new Map();
  if (activeTab?.id !== undefined) {
    tabsById.set(activeTab.id, activeTab);
  }

  const tabs = {
    onActivated,
    onUpdated,
    onRemoved,
    async get(tabId) {
      getCalls += 1;
      const tab = tabsById.get(tabId);
      if (!tab) throw new Error("missing tab");
      return tab;
    },
    async query() {
      queryCalls += 1;
      return activeTab ? [activeTab] : [];
    },
  };
  const windows = { WINDOW_ID_NONE: -1, onFocusChanged };
  const observations = {
    async submit(event) {
      events.push(structuredClone(event));
      return { accepted: true, reason: "OK" };
    },
  };
  let fatalErrors = 0;
  const collector = new NavigationVisibilityCollector({
    tabs,
    windows,
    authority,
    observations,
    events: deterministicEvents(),
    onFatalError: () => {
      fatalErrors += 1;
      authority.suspendLocalCapture();
    },
  });
  collector.install();
  return {
    collector,
    authority,
    events,
    onActivated,
    onUpdated,
    onRemoved,
    onFocusChanged,
    tabsById,
    get getCalls() { return getCalls; },
    get queryCalls() { return queryCalls; },
    get fatalErrors() { return fatalErrors; },
  };
}

async function flush() {
  await new Promise((resolve) => setTimeout(resolve, 0));
}

test("collector never reads a tab before capture authority exists", async () => {
  const authority = new CaptureAuthority();
  const h = harness({ authority, activeTab: { id: 1, windowId: 2, incognito: false, url: "https://example.test/" } });
  h.onActivated.emit({ tabId: 1, windowId: 2 });
  await flush();
  assert.equal(h.getCalls, 0);
  assert.equal(h.events.length, 0);
});

test("Start snapshot creates navigation and visibility with helper authority", async () => {
  const h = harness({
    activeTab: {
      id: 1,
      windowId: 2,
      active: true,
      incognito: false,
      url: "https://example.test/article",
      title: "Example article",
    },
  });

  const result = await h.collector.syncCurrentActiveView("session_start");
  assert.deepEqual(result, { accepted: true, reason: "OK" });
  assert.equal(h.queryCalls, 1);
  assert.equal(h.events.length, 2);
  assert.equal(h.events[0].event_type, "navigation");
  assert.equal(h.events[0].payload.transition_kind, "session_start_snapshot");
  assert.deepEqual(h.events[0].source, {
    url: "https://example.test/article",
    title: "Example article",
  });
  assert.equal(h.events[1].event_type, "visibility");
  assert.equal(h.events[1].payload.fact, "active_tab_snapshot");
  assert.equal(h.events[0].session_id, "ses_1");
  assert.equal(h.events[0].recording_epoch, 7);
  assert.equal(h.events[0].wall_time, h.events[1].wall_time);
  assert.equal(h.events[0].monotonic_ms, h.events[1].monotonic_ms);
});

test("private active tab persists continuity and privacy reason but never URL or title", async () => {
  const secretUrl = "https://secret.example/private?token=do-not-store";
  const secretTitle = "Secret private tab";
  const h = harness({
    activeTab: {
      id: 3,
      windowId: 4,
      active: true,
      incognito: true,
      url: secretUrl,
      title: secretTitle,
    },
  });

  const result = await h.collector.syncCurrentActiveView("session_start");
  assert.deepEqual(result, { accepted: true, reason: "OK" });
  assert.equal(h.events.length, 3);
  assert.equal(h.events[0].event_type, "navigation");
  assert.equal("source" in h.events[0], false);
  assert.equal(h.events[1].event_type, "privacy_decision");
  assert.equal(h.events[1].capture_mode, "blocked");
  assert.equal(h.events[1].payload.reason, "PRIVATE_BROWSING");
  assert.equal("source" in h.events[1], false);
  assert.equal(h.events[2].event_type, "visibility");
  const serialized = JSON.stringify(h.events);
  assert.equal(serialized.includes(secretUrl), false);
  assert.equal(serialized.includes(secretTitle), false);
  assert.equal(serialized.includes("do-not-store"), false);
});

test("browser-internal navigation never persists its URL", async () => {
  const internalUrl = "chrome://settings/passwords";
  const h = harness({
    activeTab: { id: 5, windowId: 1, active: true, incognito: false, url: internalUrl, title: "Passwords" },
  });
  const result = await h.collector.syncCurrentActiveView("session_start");
  assert.equal(result.accepted, true);
  assert.equal(h.events[1].event_type, "privacy_decision");
  assert.equal(h.events[1].payload.reason, "BROWSER_INTERNAL_SURFACE");
  assert.equal(JSON.stringify(h.events).includes(internalUrl), false);
  assert.equal(JSON.stringify(h.events).includes("Passwords"), false);
});

test("Pause before queued activation prevents tabs.get and persistence", async () => {
  const h = harness({
    activeTab: { id: 7, windowId: 8, active: true, incognito: false, url: "https://example.test/" },
  });
  h.onActivated.emit({ tabId: 7, windowId: 8 });
  const paused = h.authority.applyHelperState({ state: "PAUSED", sessionId: "ses_1", recordingEpoch: 8 });
  assert.equal(paused.accepted, true);
  await flush();
  assert.equal(h.getCalls, 0);
  assert.equal(h.events.length, 0);
});

test("URL change creates a fresh view without durable runtime tab IDs", async () => {
  const tab = { id: 9, windowId: 2, active: true, incognito: false, url: "https://example.test/a", title: "A" };
  const h = harness({ activeTab: tab });
  await h.collector.syncCurrentActiveView("session_start");
  h.events.length = 0;

  const updated = { ...tab, url: "https://example.test/b", title: "B" };
  h.tabsById.set(9, updated);
  h.onUpdated.emit(9, { url: updated.url }, updated);
  await flush();

  assert.equal(h.events.length, 1);
  assert.equal(h.events[0].event_type, "navigation");
  assert.equal(h.events[0].payload.transition_kind, "url_changed");
  assert.equal(h.events[0].source.url, "https://example.test/b");
  assert.notEqual(h.events[0].payload.previous_view_id, h.events[0].payload.new_view_id);
  assert.equal(JSON.stringify(h.events[0]).includes('"tabId"'), false);
  assert.equal(JSON.stringify(h.events[0]).includes('"windowId"'), false);
});

test("window focus loss remains unknown rather than claiming invisibility", async () => {
  const h = harness({
    activeTab: { id: 11, windowId: 12, active: true, incognito: false, url: "https://example.test/" },
  });
  await h.collector.syncCurrentActiveView("session_start");
  h.events.length = 0;

  h.onFocusChanged.emit(-1);
  await flush();
  assert.equal(h.events.length, 1);
  assert.equal(h.events[0].event_type, "visibility");
  assert.equal(h.events[0].payload.fact, "window_focus_lost");
  assert.equal(h.events[0].payload.exposure_visibility, "unknown");
});

test("Resume snapshot creates a new view segment across the exposure break", async () => {
  const h = harness({
    activeTab: { id: 13, windowId: 14, active: true, incognito: false, url: "https://example.test/" },
  });
  await h.collector.syncCurrentActiveView("session_start");
  const firstView = h.events[0].view_id;
  h.events.length = 0;

  const paused = h.authority.applyHelperState({ state: "PAUSED", sessionId: "ses_1", recordingEpoch: 8 });
  assert.equal(paused.accepted, true);
  const resumed = h.authority.applyHelperState({ state: "RECORDING", sessionId: "ses_1", recordingEpoch: 9 });
  assert.equal(resumed.accepted, true);
  const result = await h.collector.syncCurrentActiveView("session_resume");

  assert.equal(result.accepted, true);
  assert.equal(h.events[0].payload.transition_kind, "session_resume_snapshot");
  assert.equal(h.events[0].payload.previous_view_id, firstView);
  assert.notEqual(h.events[0].view_id, firstView);
  assert.equal(h.events[0].recording_epoch, 9);
});
