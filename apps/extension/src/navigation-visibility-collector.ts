import type { CaptureAuthority, CaptureToken } from "./capture-authority.js";
import {
  NavigationPrivacyGate,
  type NavigationContext,
  type NavigationEligibility,
} from "./navigation-privacy-gate.js";
import {
  ObservationEventFactory,
  type ObservationMoment,
} from "./observation-event.js";
import type { ObservationSubmitResult, ProtocolObservationPort } from "./protocol-observation-submit.js";

export interface BrowserTabLike {
  id?: number;
  windowId?: number;
  active?: boolean;
  incognito?: boolean;
  url?: string;
  title?: string;
}

export interface BrowserTabsEvent<T extends (...args: any[]) => void> {
  addListener(listener: T): void;
}

export interface BrowserTabsAPI {
  get(tabId: number): Promise<BrowserTabLike>;
  query(queryInfo: Record<string, unknown>): Promise<BrowserTabLike[]>;
  onActivated: BrowserTabsEvent<(activeInfo: { tabId: number; windowId: number }) => void>;
  onUpdated: BrowserTabsEvent<(
    tabId: number,
    changeInfo: { url?: string; status?: string },
    tab: BrowserTabLike,
  ) => void>;
  onRemoved: BrowserTabsEvent<(tabId: number, removeInfo: { windowId: number; isWindowClosing: boolean }) => void>;
}

export interface BrowserWindowsAPI {
  WINDOW_ID_NONE?: number;
  onFocusChanged: BrowserTabsEvent<(windowId: number) => void>;
}

export type RecordingActivationReason = "session_start" | "session_resume";

export type CollectorResult =
  | { accepted: true; reason: "OK" }
  | { accepted: false; reason: string };

export class NavigationVisibilityCollector {
  readonly privacyGate: NavigationPrivacyGate;
  #tabs: BrowserTabsAPI;
  #windows: BrowserWindowsAPI;
  #authority: CaptureAuthority;
  #observations: ProtocolObservationPort;
  #events: ObservationEventFactory;
  #onFatalError: () => void;
  #views = new Map<number, string>();
  #activeTabsByWindow = new Map<number, number>();
  #focusedWindowId: number | null = null;
  #queue: Promise<void> = Promise.resolve();
  #installed = false;

  constructor({
    tabs,
    windows,
    authority,
    observations,
    privacyGate = new NavigationPrivacyGate(),
    events = new ObservationEventFactory(),
    onFatalError = () => authority.suspendLocalCapture(),
  }: {
    tabs: BrowserTabsAPI;
    windows: BrowserWindowsAPI;
    authority: CaptureAuthority;
    observations: ProtocolObservationPort;
    privacyGate?: NavigationPrivacyGate;
    events?: ObservationEventFactory;
    onFatalError?: () => void;
  }) {
    this.#tabs = tabs;
    this.#windows = windows;
    this.#authority = authority;
    this.#observations = observations;
    this.privacyGate = privacyGate;
    this.#events = events;
    this.#onFatalError = onFatalError;
  }

  install(): void {
    if (this.#installed) {
      return;
    }
    this.#installed = true;

    this.#tabs.onActivated.addListener((info) => {
      const capture = this.#begin();
      if (!capture) {
        return;
      }
      this.#enqueue(() => this.#handleActivated(info, capture.token, capture.moment));
    });

    this.#tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
      if (typeof changeInfo.url !== "string") {
        return;
      }
      const capture = this.#begin();
      if (!capture) {
        return;
      }
      const url = changeInfo.url;
      this.#enqueue(() => this.#handleUrlChanged(tabId, url, tab, capture.token, capture.moment));
    });

    this.#tabs.onRemoved.addListener((tabId, removeInfo) => {
      const capture = this.#begin();
      const viewId = this.#views.get(tabId);
      this.#views.delete(tabId);
      this.privacyGate.exclusions.clearTab?.(tabId);
      if (this.#activeTabsByWindow.get(removeInfo.windowId) === tabId) {
        this.#activeTabsByWindow.delete(removeInfo.windowId);
      }
      if (!capture || !viewId) {
        return;
      }
      this.#enqueue(async () => {
        await this.#submitDraft(capture.token, () => this.#events.draft(
          "navigation",
          { transition_kind: "tab_closed" },
          { viewId, moment: capture.moment },
        ));
      });
    });

    this.#windows.onFocusChanged.addListener((windowId) => {
      const capture = this.#begin();
      if (!capture) {
        return;
      }
      this.#enqueue(() => this.#handleWindowFocusChanged(windowId, capture.token, capture.moment));
    });
  }

  async syncCurrentActiveView(reason: RecordingActivationReason): Promise<CollectorResult> {
    if (reason === "session_start") {
      this.#resetContinuity();
    }
    const capture = this.#begin();
    if (!capture) {
      return { accepted: false, reason: "CAPTURE_NOT_AUTHORIZED" };
    }

    return this.#enqueueResult(async () => {
      if (!this.#authority.validateCaptureToken(capture.token).valid) {
        return { accepted: false, reason: "STALE_CAPTURE_AUTHORITY" };
      }

      let tabs: BrowserTabLike[];
      try {
        tabs = await this.#tabs.query({ active: true, lastFocusedWindow: true });
      } catch {
        return this.#submitGap(capture.token, capture.moment, "ACTIVE_TAB_QUERY_FAILED");
      }
      if (!this.#authority.validateCaptureToken(capture.token).valid) {
        return { accepted: false, reason: "STALE_CAPTURE_AUTHORITY" };
      }
      const tab = tabs.find((candidate) => typeof candidate.id === "number") ?? null;
      if (!tab || typeof tab.id !== "number") {
        return this.#submitGap(capture.token, capture.moment, "ACTIVE_TAB_UNAVAILABLE");
      }

      const tabId = tab.id;
      const previousViewId = this.#views.get(tabId) ?? null;
      const viewId = this.#events.nextViewId();
      this.#views.set(tabId, viewId);
      if (typeof tab.windowId === "number") {
        this.#activeTabsByWindow.set(tab.windowId, tabId);
        this.#focusedWindowId = tab.windowId;
      }

      const navigation = await this.#submitNavigation(
        capture.token,
        capture.moment,
        tabId,
        tab,
        viewId,
        previousViewId,
        reason === "session_start" ? "session_start_snapshot" : "session_resume_snapshot",
      );
      if (!navigation.accepted) {
        return navigation;
      }
      return this.#submitDraft(capture.token, () => this.#events.draft(
        "visibility",
        { fact: "active_tab_snapshot", window_focus: "focused" },
        { viewId, moment: capture.moment },
      ));
    });
  }

  #begin(): { token: CaptureToken; moment: ObservationMoment } | null {
    const started = this.#authority.beginCapture();
    if (!started.allowed) {
      return null;
    }
    return { token: started.token, moment: this.#events.captureMoment() };
  }

  async #handleActivated(
    info: { tabId: number; windowId: number },
    token: CaptureToken,
    moment: ObservationMoment,
  ): Promise<void> {
    if (!this.#authority.validateCaptureToken(token).valid) {
      return;
    }

    const previousTabId = this.#activeTabsByWindow.get(info.windowId);
    if (previousTabId !== undefined && previousTabId !== info.tabId) {
      const previousViewId = this.#views.get(previousTabId);
      if (previousViewId) {
        const hidden = await this.#submitDraft(token, () => this.#events.draft(
          "visibility",
          { fact: "tab_backgrounded", active_in_window: false },
          { viewId: previousViewId, moment },
        ));
        if (!hidden.accepted) {
          return;
        }
      }
    }

    let tab: BrowserTabLike;
    try {
      tab = await this.#tabs.get(info.tabId);
    } catch {
      await this.#submitGap(token, moment, "TAB_LOOKUP_FAILED");
      return;
    }
    if (!this.#authority.validateCaptureToken(token).valid) {
      return;
    }

    this.#activeTabsByWindow.set(info.windowId, info.tabId);
    let viewId = this.#views.get(info.tabId);
    if (!viewId) {
      viewId = this.#events.nextViewId();
      this.#views.set(info.tabId, viewId);
      const navigation = await this.#submitNavigation(
        token,
        moment,
        info.tabId,
        tab,
        viewId,
        null,
        "view_observed",
      );
      if (!navigation.accepted) {
        return;
      }
    } else {
      const privacy = await this.#submitPrivacyIfDenied(token, moment, info.tabId, tab, viewId);
      if (!privacy.accepted) {
        return;
      }
    }

    await this.#submitDraft(token, () => this.#events.draft(
      "visibility",
      {
        fact: "tab_activated",
        active_in_window: true,
        window_focus: this.#focusedWindowId === info.windowId ? "focused" : "unknown",
      },
      { viewId, moment },
    ));
  }

  async #handleUrlChanged(
    tabId: number,
    url: string,
    tab: BrowserTabLike,
    token: CaptureToken,
    moment: ObservationMoment,
  ): Promise<void> {
    if (!this.#authority.validateCaptureToken(token).valid) {
      return;
    }
    const previousViewId = this.#views.get(tabId) ?? null;
    const viewId = this.#events.nextViewId();
    this.#views.set(tabId, viewId);
    await this.#submitNavigation(
      token,
      moment,
      tabId,
      { ...tab, url },
      viewId,
      previousViewId,
      "url_changed",
    );
  }

  async #handleWindowFocusChanged(
    windowId: number,
    token: CaptureToken,
    moment: ObservationMoment,
  ): Promise<void> {
    if (!this.#authority.validateCaptureToken(token).valid) {
      return;
    }
    const none = this.#windows.WINDOW_ID_NONE ?? -1;
    if (windowId === none) {
      const previousWindowId = this.#focusedWindowId;
      this.#focusedWindowId = null;
      if (previousWindowId === null) {
        return;
      }
      const activeTabId = this.#activeTabsByWindow.get(previousWindowId);
      const viewId = activeTabId === undefined ? undefined : this.#views.get(activeTabId);
      if (!viewId) {
        return;
      }
      await this.#submitDraft(token, () => this.#events.draft(
        "visibility",
        {
          fact: "window_focus_lost",
          window_focus: "not_focused",
          exposure_visibility: "unknown",
        },
        { viewId, moment },
      ));
      return;
    }

    this.#focusedWindowId = windowId;
    const activeTabId = this.#activeTabsByWindow.get(windowId);
    const viewId = activeTabId === undefined ? undefined : this.#views.get(activeTabId);
    if (!viewId) {
      return;
    }
    await this.#submitDraft(token, () => this.#events.draft(
      "visibility",
      { fact: "window_focused", window_focus: "focused" },
      { viewId, moment },
    ));
  }

  async #submitNavigation(
    token: CaptureToken,
    moment: ObservationMoment,
    tabId: number,
    tab: BrowserTabLike,
    viewId: string,
    previousViewId: string | null,
    transitionKind: string,
  ): Promise<CollectorResult> {
    const eligibility = this.privacyGate.evaluate(toNavigationContext(tabId, tab));
    const continuity = await this.#submitDraft(token, () => this.#events.draft(
      "navigation",
      {
        transition_kind: transitionKind,
        previous_view_id: previousViewId,
        new_view_id: viewId,
        source_status: eligibility.allowed
          ? eligibility.source ? "allowed" : eligibility.sourceOmittedReason ?? "unavailable"
          : "blocked",
      },
      {
        viewId,
        ...(eligibility.allowed && eligibility.source ? { source: eligibility.source } : {}),
        moment,
      },
    ));
    if (!continuity.accepted) {
      return continuity;
    }
    if (eligibility.allowed) {
      return { accepted: true, reason: "OK" };
    }
    return this.#submitPrivacyDecision(token, moment, viewId, eligibility);
  }

  async #submitPrivacyIfDenied(
    token: CaptureToken,
    moment: ObservationMoment,
    tabId: number,
    tab: BrowserTabLike,
    viewId: string,
  ): Promise<CollectorResult> {
    const eligibility = this.privacyGate.evaluate(toNavigationContext(tabId, tab));
    if (eligibility.allowed) {
      return { accepted: true, reason: "OK" };
    }
    return this.#submitPrivacyDecision(token, moment, viewId, eligibility);
  }

  #submitPrivacyDecision(
    token: CaptureToken,
    moment: ObservationMoment,
    viewId: string,
    eligibility: Exclude<NavigationEligibility, { allowed: true }>,
  ): Promise<CollectorResult> {
    return this.#submitDraft(token, () => this.#events.draft(
      "privacy_decision",
      {
        reason: eligibility.reason,
        ...(eligibility.ruleId ? { rule_id: eligibility.ruleId } : {}),
      },
      { viewId, captureMode: "blocked", moment },
    ));
  }

  #submitGap(token: CaptureToken, moment: ObservationMoment, reason: string): Promise<CollectorResult> {
    return this.#submitDraft(token, () => this.#events.draft(
      "gap_error",
      { reason },
      { moment },
    ));
  }

  async #submitDraft(
    token: CaptureToken,
    build: () => Record<string, unknown>,
  ): Promise<CollectorResult> {
    const finalized = this.#authority.finalizeCapture(token, build);
    if (!finalized.accepted) {
      return { accepted: false, reason: finalized.reason };
    }
    const result: ObservationSubmitResult = await this.#observations.submit(finalized.observation, token);
    if (!result.accepted && !this.#authority.captureAllowed) {
      this.#resetContinuity();
    }
    return result;
  }

  #enqueue(work: () => Promise<void>): void {
    this.#queue = this.#queue.then(work, work).catch(() => {
      this.#resetContinuity();
      this.#onFatalError();
    });
  }

  #enqueueResult(work: () => Promise<CollectorResult>): Promise<CollectorResult> {
    let resolveResult!: (result: CollectorResult) => void;
    const result = new Promise<CollectorResult>((resolve) => {
      resolveResult = resolve;
    });
    this.#queue = this.#queue.then(async () => {
      try {
        resolveResult(await work());
      } catch {
        this.#resetContinuity();
        this.#onFatalError();
        resolveResult({ accepted: false, reason: "COLLECTOR_INTERNAL_ERROR" });
      }
    }, async () => {
      try {
        resolveResult(await work());
      } catch {
        this.#resetContinuity();
        this.#onFatalError();
        resolveResult({ accepted: false, reason: "COLLECTOR_INTERNAL_ERROR" });
      }
    });
    return result;
  }

  #resetContinuity(): void {
    this.#views.clear();
    this.#activeTabsByWindow.clear();
    this.#focusedWindowId = null;
  }
}

function toNavigationContext(tabId: number, tab: BrowserTabLike): NavigationContext {
  const context: NavigationContext = { tabId };
  if (typeof tab.incognito === "boolean") {
    context.incognito = tab.incognito;
  }
  if (typeof tab.url === "string") {
    context.url = tab.url;
  }
  if (typeof tab.title === "string") {
    context.title = tab.title;
  }
  return context;
}
