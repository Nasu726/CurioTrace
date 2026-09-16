export const REQUIRED_HOST_ORIGINS = Object.freeze([
  "http://*/*",
  "https://*/*",
] as const);

export const HOST_PERMISSION_EXPLANATION = Object.freeze({
  title: "Allow website access for recording",
  body:
    "CurioTrace needs access to normal web pages while a recording session is active so it can keep the same session complete as you move between sites and tabs. Granting this browser permission does not start background monitoring: CurioTrace reads browsing content only after you press Start and stops while Paused or after Stop. Private and excluded pages remain outside capture under the privacy policy. This permission also does not authorize sending your browsing data to an external AI service.",
  primaryAction: "Continue to browser permission",
  secondaryAction: "Cancel",
  denialMessage:
    "Recording was not started because CurioTrace does not create partial cross-site sessions without the required website access. You can try Start again later.",
});

export interface HostPermissionPort {
  contains(origins: readonly string[]): Promise<boolean>;
  request(origins: readonly string[]): Promise<boolean>;
}

export type SessionStartResult =
  | { accepted: true; reason: "OK" }
  | { accepted: false; reason: string };

export interface SessionStartPort {
  startSession(): Promise<SessionStartResult>;
}

export type StartFlowPhase =
  | "READY"
  | "CHECKING_PERMISSION"
  | "AWAITING_EXPLANATION"
  | "REQUESTING_PERMISSION"
  | "STARTING_SESSION";

export type StartFlowResult =
  | {
      kind: "EXPLANATION_REQUIRED";
      explanation: typeof HOST_PERMISSION_EXPLANATION;
    }
  | { kind: "STARTED" }
  | {
      kind: "NOT_STARTED";
      reason:
        | "FLOW_BUSY"
        | "EXPLANATION_NOT_PENDING"
        | "USER_CANCELLED"
        | "PERMISSION_DENIED"
        | "PERMISSION_CHECK_FAILED"
        | "PERMISSION_REQUEST_FAILED"
        | "PERMISSION_NOT_EFFECTIVE"
        | "SESSION_START_FAILED"
        | "SESSION_START_REJECTED";
      detail?: string;
    };

export class PermissionStartController {
  #permissions: HostPermissionPort;
  #session: SessionStartPort;
  #phase: StartFlowPhase = "READY";

  constructor({ permissions, session }: { permissions: HostPermissionPort; session: SessionStartPort }) {
    this.#permissions = permissions;
    this.#session = session;
  }

  get snapshot() {
    return Object.freeze({ phase: this.#phase });
  }

  async beginStart(): Promise<StartFlowResult> {
    if (this.#phase !== "READY") {
      return notStarted("FLOW_BUSY");
    }

    this.#phase = "CHECKING_PERMISSION";
    let granted: boolean;
    try {
      granted = await this.#permissions.contains(REQUIRED_HOST_ORIGINS);
    } catch (error) {
      this.#phase = "READY";
      return notStarted("PERMISSION_CHECK_FAILED", errorDetail(error));
    }

    if (!granted) {
      this.#phase = "AWAITING_EXPLANATION";
      return {
        kind: "EXPLANATION_REQUIRED",
        explanation: HOST_PERMISSION_EXPLANATION,
      };
    }

    return this.#startIfPermissionStillGranted();
  }

  // This method is intended to be called directly from the user's Continue
  // action. Browser permission APIs require runtime permission requests to be
  // initiated from a user gesture on supported browsers.
  async continueAfterExplanation(): Promise<StartFlowResult> {
    if (this.#phase !== "AWAITING_EXPLANATION") {
      return notStarted("EXPLANATION_NOT_PENDING");
    }

    this.#phase = "REQUESTING_PERMISSION";
    let granted: boolean;
    try {
      // Call request before any unrelated await so the browser adapter can keep
      // this operation directly attached to the user's action handler.
      const request = this.#permissions.request(REQUIRED_HOST_ORIGINS);
      granted = await request;
    } catch (error) {
      this.#phase = "READY";
      return notStarted("PERMISSION_REQUEST_FAILED", errorDetail(error));
    }

    if (!granted) {
      this.#phase = "READY";
      return notStarted("PERMISSION_DENIED");
    }

    return this.#startIfPermissionStillGranted();
  }

  cancelExplanation(): StartFlowResult {
    if (this.#phase !== "AWAITING_EXPLANATION") {
      return notStarted("EXPLANATION_NOT_PENDING");
    }
    this.#phase = "READY";
    return notStarted("USER_CANCELLED");
  }

  async #startIfPermissionStillGranted(): Promise<StartFlowResult> {
    this.#phase = "CHECKING_PERMISSION";
    try {
      if (!(await this.#permissions.contains(REQUIRED_HOST_ORIGINS))) {
        this.#phase = "READY";
        return notStarted("PERMISSION_NOT_EFFECTIVE");
      }
    } catch (error) {
      this.#phase = "READY";
      return notStarted("PERMISSION_CHECK_FAILED", errorDetail(error));
    }

    this.#phase = "STARTING_SESSION";
    let result: SessionStartResult;
    try {
      result = await this.#session.startSession();
    } catch (error) {
      this.#phase = "READY";
      return notStarted("SESSION_START_FAILED", errorDetail(error));
    }

    this.#phase = "READY";
    if (!result.accepted) {
      return notStarted("SESSION_START_REJECTED", result.reason);
    }
    return { kind: "STARTED" };
  }
}

export interface WebExtensionPermissionsAPI {
  contains(permission: { origins: string[] }): Promise<boolean>;
  request(permission: { origins: string[] }): Promise<boolean>;
}

// Thin Promise-based adapter shared by current Chromium and Firefox MV3
// permission APIs. Manifest declaration remains a separate bootstrap concern.
export class WebExtensionHostPermissionPort implements HostPermissionPort {
  #api: WebExtensionPermissionsAPI;

  constructor(api: WebExtensionPermissionsAPI) {
    this.#api = api;
  }

  contains(origins: readonly string[]): Promise<boolean> {
    return this.#api.contains({ origins: [...origins] });
  }

  request(origins: readonly string[]): Promise<boolean> {
    return this.#api.request({ origins: [...origins] });
  }
}

function notStarted(reason: Extract<StartFlowResult, { kind: "NOT_STARTED" }>['reason'], detail?: string): StartFlowResult {
  if (detail === undefined) {
    return { kind: "NOT_STARTED", reason };
  }
  return { kind: "NOT_STARTED", reason, detail };
}

function errorDetail(error: unknown): string | undefined {
  if (error instanceof Error && error.message.length > 0) {
    return error.message;
  }
  return undefined;
}
