import assert from "node:assert/strict";
import test from "node:test";

import {
  HOST_PERMISSION_EXPLANATION,
  PermissionStartController,
  REQUIRED_HOST_ORIGINS,
  WebExtensionHostPermissionPort,
} from "../dist/permission-controller.js";

class FakePermissions {
  constructor({ granted = false, requestResult = false } = {}) {
    this.granted = granted;
    this.requestResult = requestResult;
    this.containsCalls = [];
    this.requestCalls = [];
    this.containsError = null;
    this.requestError = null;
  }

  async contains(origins) {
    this.containsCalls.push([...origins]);
    if (this.containsError) throw this.containsError;
    return this.granted;
  }

  async request(origins) {
    this.requestCalls.push([...origins]);
    if (this.requestError) throw this.requestError;
    if (this.requestResult) this.granted = true;
    return this.requestResult;
  }
}

class FakeSession {
  constructor(result = { accepted: true, reason: "OK" }) {
    this.result = result;
    this.calls = 0;
    this.error = null;
  }

  async startSession() {
    this.calls += 1;
    if (this.error) throw this.error;
    return this.result;
  }
}

function controller(permissions, session = new FakeSession()) {
  return { controller: new PermissionStartController({ permissions, session }), session };
}

test("missing host permission requires CurioTrace explanation before browser request", async () => {
  const permissions = new FakePermissions();
  const { controller: flow, session } = controller(permissions);

  const result = await flow.beginStart();
  assert.equal(result.kind, "EXPLANATION_REQUIRED");
  assert.equal(result.explanation, HOST_PERMISSION_EXPLANATION);
  assert.equal(flow.snapshot.phase, "AWAITING_EXPLANATION");
  assert.equal(permissions.requestCalls.length, 0);
  assert.equal(session.calls, 0);
});

test("permission denial never calls helper Start and returns to ready", async () => {
  const permissions = new FakePermissions({ requestResult: false });
  const { controller: flow, session } = controller(permissions);
  await flow.beginStart();

  const result = await flow.continueAfterExplanation();
  assert.deepEqual(result, { kind: "NOT_STARTED", reason: "PERMISSION_DENIED" });
  assert.equal(permissions.requestCalls.length, 1);
  assert.equal(session.calls, 0);
  assert.equal(flow.snapshot.phase, "READY");
});

test("granted permission is verified before helper Start", async () => {
  const permissions = new FakePermissions({ requestResult: true });
  const { controller: flow, session } = controller(permissions);
  await flow.beginStart();

  const result = await flow.continueAfterExplanation();
  assert.deepEqual(result, { kind: "STARTED" });
  assert.equal(permissions.requestCalls.length, 1);
  assert.equal(permissions.containsCalls.length, 2);
  assert.equal(session.calls, 1);
  assert.equal(flow.snapshot.phase, "READY");
});

test("already-granted permission skips explanation and browser request", async () => {
  const permissions = new FakePermissions({ granted: true });
  const { controller: flow, session } = controller(permissions);

  const result = await flow.beginStart();
  assert.deepEqual(result, { kind: "STARTED" });
  assert.equal(permissions.requestCalls.length, 0);
  assert.equal(permissions.containsCalls.length, 2);
  assert.equal(session.calls, 1);
});

test("permission that is not effective after a positive request cannot start", async () => {
  const permissions = new FakePermissions({ requestResult: true });
  permissions.request = async function request(origins) {
    this.requestCalls.push([...origins]);
    return true;
  };
  const { controller: flow, session } = controller(permissions);
  await flow.beginStart();

  const result = await flow.continueAfterExplanation();
  assert.deepEqual(result, { kind: "NOT_STARTED", reason: "PERMISSION_NOT_EFFECTIVE" });
  assert.equal(session.calls, 0);
});

test("cancelled explanation never opens browser permission or helper Start", async () => {
  const permissions = new FakePermissions();
  const { controller: flow, session } = controller(permissions);
  await flow.beginStart();

  const result = flow.cancelExplanation();
  assert.deepEqual(result, { kind: "NOT_STARTED", reason: "USER_CANCELLED" });
  assert.equal(permissions.requestCalls.length, 0);
  assert.equal(session.calls, 0);
  assert.equal(flow.snapshot.phase, "READY");
});

test("browser permission errors fail closed", async () => {
  const permissions = new FakePermissions();
  permissions.containsError = new Error("contains failed");
  const first = controller(permissions);
  const checkResult = await first.controller.beginStart();
  assert.equal(checkResult.kind, "NOT_STARTED");
  assert.equal(checkResult.reason, "PERMISSION_CHECK_FAILED");
  assert.equal(first.session.calls, 0);

  const requestPermissions = new FakePermissions();
  requestPermissions.requestError = new Error("request failed");
  const second = controller(requestPermissions);
  await second.controller.beginStart();
  const requestResult = await second.controller.continueAfterExplanation();
  assert.equal(requestResult.kind, "NOT_STARTED");
  assert.equal(requestResult.reason, "PERMISSION_REQUEST_FAILED");
  assert.equal(second.session.calls, 0);
});

test("helper rejection does not report a started recording", async () => {
  const permissions = new FakePermissions({ granted: true });
  const session = new FakeSession({ accepted: false, reason: "STORE_UNAVAILABLE" });
  const flow = new PermissionStartController({ permissions, session });

  const result = await flow.beginStart();
  assert.deepEqual(result, {
    kind: "NOT_STARTED",
    reason: "SESSION_START_REJECTED",
    detail: "STORE_UNAVAILABLE",
  });
  assert.equal(flow.snapshot.phase, "READY");
});

test("concurrent Start attempts are rejected while permission state is unresolved", async () => {
  let resolveContains;
  const permissions = {
    containsCalls: 0,
    async contains() {
      this.containsCalls += 1;
      return new Promise((resolve) => {
        resolveContains = resolve;
      });
    },
    async request() {
      throw new Error("unexpected request");
    },
  };
  const session = new FakeSession();
  const flow = new PermissionStartController({ permissions, session });

  const first = flow.beginStart();
  const second = await flow.beginStart();
  assert.deepEqual(second, { kind: "NOT_STARTED", reason: "FLOW_BUSY" });
  resolveContains(false);
  const firstResult = await first;
  assert.equal(firstResult.kind, "EXPLANATION_REQUIRED");
  assert.equal(session.calls, 0);
});

test("webextension adapter requests exactly HTTP and HTTPS optional hosts", async () => {
  const seen = [];
  const api = {
    async contains(permission) {
      seen.push(["contains", permission]);
      return true;
    },
    async request(permission) {
      seen.push(["request", permission]);
      return true;
    },
  };
  const port = new WebExtensionHostPermissionPort(api);

  assert.equal(await port.contains(REQUIRED_HOST_ORIGINS), true);
  assert.equal(await port.request(REQUIRED_HOST_ORIGINS), true);
  assert.deepEqual(seen, [
    ["contains", { origins: ["http://*/*", "https://*/*"] }],
    ["request", { origins: ["http://*/*", "https://*/*"] }],
  ]);
});
