import { BackgroundSessionBroker, type ExtensionMessageSender } from "./background-broker.js";
import { HelperConnectionController } from "./helper-connection.js";
import {
  NavigationVisibilityCollector,
  type BrowserTabsAPI,
  type BrowserWindowsAPI,
} from "./navigation-visibility-collector.js";
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
  tabs: BrowserTabsAPI;
  windows: BrowserWindowsAPI;
}

export function installBackground(
  api: BackgroundExtensionAPI,
  { hostName = NATIVE_HOST_NAME }: { hostName?: string } = {},
) {
  const helper = new HelperConnectionController({
    runtime: api.runtime,
    hostName,
  });
  const permissions = new WebExtensionHostPermissionPort(api.permissions);
  const observations = new ProtocolObservationPort({
    protocol: helper.protocol,
    transport: helper,
    onTerminalFailure: () => helper.disconnect(),
  });
  const collector = new NavigationVisibilityCollector({
    tabs: api.tabs,
    windows: api.windows,
    authority: helper.protocol.authority,
    observations,
    onFatalError: () => helper.disconnect(),
  });
  collector.install();

  const broker = new BackgroundSessionBroker({
    runtimeId: api.runtime.id,
    permissions,
    helper,
    recordingObserver: collector,
  });

  api.runtime.onMessage.addListener((message, sender, sendResponse) => {
    void broker.handle(message, sender).then(
      (response) => sendResponse(response),
      () => sendResponse({ accepted: false, reason: "BACKGROUND_INTERNAL_ERROR" }),
    );
    return true;
  });

  return Object.freeze({ helper, broker, collector });
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
    !api.tabs.onUpdated ||
    !api.tabs.onRemoved ||
    !api.windows ||
    !api.windows.onFocusChanged
  ) {
    return null;
  }
  return api;
}

const detectedAPI = detectExtensionAPI();
if (detectedAPI) {
  installBackground(detectedAPI);
}
