import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import os from "node:os";
import test from "node:test";

const littleEndian = os.endianness() === "LE";

function encodeNativeMessage(message) {
  const payload = Buffer.from(JSON.stringify(message), "utf8");
  const header = Buffer.alloc(4);
  if (littleEndian) header.writeUInt32LE(payload.length, 0);
  else header.writeUInt32BE(payload.length, 0);
  return Buffer.concat([header, payload]);
}

class NativeMessageReader {
  constructor(stream) {
    this.buffer = Buffer.alloc(0);
    this.waiters = [];
    this.error = null;

    stream.on("data", (chunk) => {
      this.buffer = Buffer.concat([this.buffer, chunk]);
      this.#drain();
    });
    stream.on("error", (error) => {
      this.error = error;
      this.#drain();
    });
    stream.on("end", () => {
      if (!this.error) this.error = new Error("native host stdout ended");
      this.#drain();
    });
  }

  read() {
    const immediate = this.#tryRead();
    if (immediate) return Promise.resolve(immediate);
    if (this.error) return Promise.reject(this.error);
    return new Promise((resolve, reject) => this.waiters.push({ resolve, reject }));
  }

  #tryRead() {
    if (this.buffer.length < 4) return null;
    const length = littleEndian ? this.buffer.readUInt32LE(0) : this.buffer.readUInt32BE(0);
    if (this.buffer.length < 4 + length) return null;
    const payload = this.buffer.subarray(4, 4 + length);
    this.buffer = this.buffer.subarray(4 + length);
    return JSON.parse(payload.toString("utf8"));
  }

  #drain() {
    while (this.waiters.length) {
      const value = this.#tryRead();
      if (!value) break;
      this.waiters.shift().resolve(value);
    }
    if (this.error && this.waiters.length) {
      for (const waiter of this.waiters.splice(0)) waiter.reject(this.error);
    }
  }
}

function request(host, reader, message) {
  host.stdin.write(encodeNativeMessage(message));
  return reader.read();
}

function observation(sessionId, epoch, eventId, payload = null, captureMode = "dom") {
  return {
    schema_version: "1.0",
    event_id: eventId,
    session_id: sessionId,
    recording_epoch: epoch,
    event_type: "content_observation",
    wall_time: "2026-09-16T09:30:00Z",
    monotonic_ms: 1000,
    browser_instance_id: "br_node_integration",
    view_id: "view_integration",
    capture_mode: captureMode,
    source: { url: "https://example.test/", title: "Integration fixture" },
    payload:
      payload ?? {
        units: [{ unit_id: "u1", kind: "text", text: "allowed integration text", confidence: 1 }],
        capture_method_version: "integration/1",
      },
  };
}

test("Node extension framing interoperates with Python native helper", async (t) => {
  const host = spawn("python3", ["-m", "spikes.native_helper_harness.server"], {
    cwd: process.cwd(),
    stdio: ["pipe", "pipe", "pipe"],
  });
  const reader = new NativeMessageReader(host.stdout);
  let stderr = "";
  host.stderr.setEncoding("utf8");
  host.stderr.on("data", (chunk) => {
    stderr += chunk;
  });

  t.after(async () => {
    if (!host.killed) host.stdin.end();
    await new Promise((resolve) => {
      if (host.exitCode !== null) return resolve();
      host.once("exit", resolve);
      setTimeout(() => {
        if (host.exitCode === null) host.kill("SIGKILL");
        resolve();
      }, 1000).unref();
    });
    assert.equal(stderr, "", `native host wrote unexpected stderr: ${stderr}`);
  });

  const hello = await request(host, reader, {
    protocol_version: "1.0",
    message_id: "m1",
    kind: "hello",
    payload: {
      extension_version: "integration-test",
      capabilities: ["observation_schema_v1", "recording_epoch_v1"],
    },
  });
  assert.equal(hello.kind, "hello.ack");
  assert.equal(hello.payload.compatible, true);
  assert.equal(hello.payload.session_state, "IDLE");

  const start = await request(host, reader, {
    protocol_version: "1.0",
    message_id: "m2",
    kind: "session.start",
    payload: {},
  });
  assert.equal(start.kind, "ack");
  assert.equal(start.payload.accepted, true);
  assert.equal(start.payload.state, "RECORDING");
  const sessionId = start.payload.session_id;
  const recordingEpoch = start.payload.recording_epoch;

  const acceptedObservation = await request(host, reader, {
    protocol_version: "1.0",
    message_id: "m3",
    kind: "observation.submit",
    session_id: sessionId,
    recording_epoch: recordingEpoch,
    payload: { event: observation(sessionId, recordingEpoch, "evt_ok") },
  });
  assert.equal(acceptedObservation.payload.accepted, true);

  const secret = "GOLDEN_NATIVE_REJECTION_SECRET_72B1";
  const invalidFingerprint = await request(host, reader, {
    protocol_version: "1.0",
    message_id: "m4",
    kind: "observation.submit",
    session_id: sessionId,
    recording_epoch: recordingEpoch,
    payload: {
      event: observation(
        sessionId,
        recordingEpoch,
        "evt_bad",
        { visual_fingerprint: "phash-v1:abc", ocr_text: secret },
        "fingerprint_only"
      ),
    },
  });
  assert.equal(invalidFingerprint.payload.accepted, false);
  assert.equal(JSON.stringify(invalidFingerprint).includes(secret), false);

  const pause = await request(host, reader, {
    protocol_version: "1.0",
    message_id: "m5",
    kind: "session.pause",
    session_id: sessionId,
    recording_epoch: recordingEpoch,
    payload: {},
  });
  assert.equal(pause.payload.accepted, true);
  assert.equal(pause.payload.state, "PAUSED");
  assert.ok(pause.payload.recording_epoch > recordingEpoch);

  const stale = await request(host, reader, {
    protocol_version: "1.0",
    message_id: "m6",
    kind: "observation.submit",
    session_id: sessionId,
    recording_epoch: recordingEpoch,
    payload: { event: observation(sessionId, recordingEpoch, "evt_stale") },
  });
  assert.equal(stale.payload.accepted, false);
  assert.ok(stale.payload.errors.includes("STALE_EPOCH"));
  assert.ok(stale.payload.errors.includes("NOT_RECORDING"));
});
