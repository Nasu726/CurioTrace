import { BackgroundSessionBroker, type ExtensionMessageSender } from "./background-broker.js";
import {
  BrowserObservationEventGate,
  type RawBrowserTabsAPI,
  type RawBrowserWindowsAPI,
} from "./browser-observation-event-gate.js";
import { HelperConnectionController } from "./helper-connection.js";
import { NavigationVisibilityCollector } from "./navigation-visibility-collector.js";
import {
  WebExtensionHostPermissionPort,
  type WebExtensionPermissionsAPI,
} from "./permission-controller.js";
import type { NativePortLike, NativeRuntimeLike } from "./native-messaging-transport.js";
import { ProtocolObservationPort } from "./protocol-observation-submit.js";

export const NATIVE_HOST_NAME = "uk.nasu.curiotrace";

export interface BackgroundMessageEvent {
  addListener(
    listener: (
      message: unknown,
      sender: ExtensionMessageSender,
      sendResponse: (response: unknown) => void,
    ) => boolean | void,
  ): void;
}

export interface BackgroundRuntimeAPI extends NativeRuntimeLike {
  id: string;
  onMessage: BackgroundMessageEvent;
  connectNative(application: string): NativePortLike;
}

export interface BackgroundExtensionAPI {
  runtime: BackgroundRuntimeAPI;
  permissions: WebExtensionPermissionsAPI;
  tabs: RawBrowserTabsAPI;
  windows: RawBrowserWindowsAPI;
}

export function installBackground(
  api: BackgroundExtensionAPI,
  { hostName = NATIVE_HOST_NAME }: { hostName?: string } = {},
) {
  const browserEvents = new BrowserObservationEventGate(api.tabs, api.windows);
  let collector: NavigationVisibilityCollector | null = null;

  const helper = new HelperConnectionController({
    runtime: api.runtime,
    hostName,
    onDisconnected: () => browserEvents.deactivate(),
  });
  const permissions = new WebExtensionHostPermissionPort(api.permissions);
  const observations = new ProtocolObservationPort({
    protocol: helper.protocol,
    transport: helper,
    onTerminalFailure: () => {
      browserEvents.deactivate();
      helper.disconnect();
    },
  });
  collector = new NavigationVisibilityCollector({
    tabs: browserEvents.tabs,
    windows: browserEvents.windows,
    authority: helper.protocol.authority,
    observations,
    onFatalError: () => {
      browserEvents.deactivate();
      helper.disconnect();
    },
  });
  collector.install();

  const recordingObserver = {
    async syncCurrentActiveView(reason: "session_start" | "session_resume") {
      browserEvents.activate();
      const result = await collector!.syncCurrentActiveView(reason);
      if (!result.accepted) {
        browserEvents.deactivate();
      }
      return result;
    },
    suspendObservation() {
      browserEvents.deactivate();
    },
  };

  const broker = new BackgroundSessionBroker({
    runtimeId: api.runtime.id,
    permissions,
    helper,
    recordingObserver,
  });

  api.runtime.onMessage.addListener((message, sender, sendResponse) => {
    void broker.handle(message, sender).then(
      (response) => sendResponse(response),
      () => sendResponse({ accepted: false, reason: "BACKGROUND_INTERNAL_ERROR" }),
    );
    return true;
  });

  return Object.freeze({ helper, broker, collector, browserEvents });
}

function detectExtensionAPI(): BackgroundExtensionAPI | null {
  const scope = globalThis as typeof globalThis & {
    browser?: BackgroundExtensionAPI;
    chrome?: BackgroundExtensionAPI;
  };
  const api = scope.browser ?? scope.chrome;
  if (
    !api?.runtime?.id ||
    !api.runtime.onMessage ||
    !api.permissions ||
    !api.tabs ||
    !api.tabs.onActivated ||
    typeof api.tabs.onActivated.removeListener !== "function" ||
    !api.tabs.onUpdated ||
    typeof api.tabs.onUpdated.removeListener !== "function" ||
    !api.tabs.onRemoved ||
    typeof api.tabs.onRemoved.removeListener !== "function" ||
    !api.windows ||
    !api.windows.onFocusChanged ||
    typeof api.windows.onFocusChanged.removeListener !== "function"
  ) {
    return null;
  }
  return api;
}

const detectedAPI = detectExtensionAPI();
if (detectedAPI) {
  installBackground(detectedAPI);
}
