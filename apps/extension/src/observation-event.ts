export interface ObservationClock {
  wallTime(): string;
  monotonicMs(): number;
}

export interface ObservationIdFactory {
  next(prefix: "evt" | "view" | "br"): string;
}

export interface ObservationSource {
  url: string;
  title?: string;
}

export interface ObservationDraftOptions {
  viewId?: string;
  source?: ObservationSource;
  captureMode?: "dom" | "redacted_visual" | "fingerprint_only" | "metadata_only" | "blocked" | "failed";
}

export class DefaultObservationClock implements ObservationClock {
  wallTime(): string {
    return new Date().toISOString();
  }

  monotonicMs(): number {
    if (typeof performance !== "undefined") {
      return performance.timeOrigin + performance.now();
    }
    return Date.now();
  }
}

export class RandomObservationIdFactory implements ObservationIdFactory {
  next(prefix: "evt" | "view" | "br"): string {
    return `${prefix}_${randomUuid()}`;
  }
}

export class ObservationEventFactory {
  readonly browserInstanceId: string;
  #clock: ObservationClock;
  #ids: ObservationIdFactory;

  constructor({
    clock = new DefaultObservationClock(),
    ids = new RandomObservationIdFactory(),
  }: {
    clock?: ObservationClock;
    ids?: ObservationIdFactory;
  } = {}) {
    this.#clock = clock;
    this.#ids = ids;
    this.browserInstanceId = this.#ids.next("br");
  }

  nextViewId(): string {
    return this.#ids.next("view");
  }

  draft(
    eventType: "navigation" | "visibility" | "privacy_decision" | "gap_error",
    payload: Record<string, unknown>,
    options: ObservationDraftOptions = {},
  ): Record<string, unknown> {
    const event: Record<string, unknown> = {
      schema_version: "1.0",
      event_id: this.#ids.next("evt"),
      event_type: eventType,
      wall_time: this.#clock.wallTime(),
      monotonic_ms: this.#clock.monotonicMs(),
      browser_instance_id: this.browserInstanceId,
      payload,
    };
    if (options.viewId) {
      event.view_id = options.viewId;
    }
    if (options.source) {
      event.source = options.source;
    }
    if (options.captureMode) {
      event.capture_mode = options.captureMode;
    }
    return event;
  }
}

function randomUuid(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  const bytes = new Uint8Array(16);
  if (typeof crypto !== "undefined" && typeof crypto.getRandomValues === "function") {
    crypto.getRandomValues(bytes);
  } else {
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] = Math.floor(Math.random() * 256);
    }
  }
  return [...bytes].map((value) => value.toString(16).padStart(2, "0")).join("");
}
