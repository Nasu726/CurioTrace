import type {
  BrowserTabLike,
  BrowserTabsAPI,
  BrowserTabsEvent,
  BrowserWindowsAPI,
} from "./navigation-visibility-collector.js";

type ActivatedListener = (activeInfo: { tabId: number; windowId: number }) => void;
type UpdatedListener = (
  tabId: number,
  changeInfo: { url?: string; status?: string },
  tab: BrowserTabLike,
) => void;
type RemovedListener = (tabId: number, removeInfo: { windowId: number; isWindowClosing: boolean }) => void;
type FocusChangedListener = (windowId: number) => void;

export interface RemovableBrowserEvent<Listener extends (...args: any[]) => void> {
  addListener(listener: Listener): void;
  removeListener(listener: Listener): void;
}

export interface RawBrowserTabsAPI {
  get(tabId: number): Promise<BrowserTabLike>;
  query(queryInfo: Record<string, unknown>): Promise<BrowserTabLike[]>;
  onActivated: RemovableBrowserEvent<ActivatedListener>;
  onUpdated: RemovableBrowserEvent<UpdatedListener>;
  onRemoved: RemovableBrowserEvent<RemovedListener>;
}

export interface RawBrowserWindowsAPI {
  WINDOW_ID_NONE?: number;
  onFocusChanged: RemovableBrowserEvent<FocusChangedListener>;
}

class DeferredBrowserEvent<Listener extends (...args: any[]) => void> implements BrowserTabsEvent<Listener> {
  #source: RemovableBrowserEvent<Listener>;
  #listeners = new Set<Listener>();
  #active = false;

  constructor(source: RemovableBrowserEvent<Listener>) {
    this.#source = source;
  }

  addListener(listener: Listener): void {
    if (this.#listeners.has(listener)) {
      return;
    }
    this.#listeners.add(listener);
    if (this.#active) {
      this.#source.addListener(listener);
    }
  }

  activate(): void {
    if (this.#active) {
      return;
    }
    this.#active = true;
    for (const listener of this.#listeners) {
      this.#source.addListener(listener);
    }
  }

  deactivate(): void {
    if (!this.#active) {
      return;
    }
    this.#active = false;
    for (const listener of this.#listeners) {
      this.#source.removeListener(listener);
    }
  }

  get active(): boolean {
    return this.#active;
  }
}

export class BrowserObservationEventGate {
  readonly tabs: BrowserTabsAPI;
  readonly windows: BrowserWindowsAPI;

  #activated: DeferredBrowserEvent<ActivatedListener>;
  #updated: DeferredBrowserEvent<UpdatedListener>;
  #removed: DeferredBrowserEvent<RemovedListener>;
  #focusChanged: DeferredBrowserEvent<FocusChangedListener>;

  constructor(tabs: RawBrowserTabsAPI, windows: RawBrowserWindowsAPI) {
    this.#activated = new DeferredBrowserEvent(tabs.onActivated);
    this.#updated = new DeferredBrowserEvent(tabs.onUpdated);
    this.#removed = new DeferredBrowserEvent(tabs.onRemoved);
    this.#focusChanged = new DeferredBrowserEvent(windows.onFocusChanged);

    this.tabs = Object.freeze({
      get: (tabId: number) => tabs.get(tabId),
      query: (queryInfo: Record<string, unknown>) => tabs.query(queryInfo),
      onActivated: this.#activated,
      onUpdated: this.#updated,
      onRemoved: this.#removed,
    });

    this.windows = Object.freeze({
      ...(typeof windows.WINDOW_ID_NONE === "number" ? { WINDOW_ID_NONE: windows.WINDOW_ID_NONE } : {}),
      onFocusChanged: this.#focusChanged,
    });
  }

  get active(): boolean {
    return this.#activated.active;
  }

  activate(): void {
    this.#activated.activate();
    this.#updated.activate();
    this.#removed.activate();
    this.#focusChanged.activate();
  }

  deactivate(): void {
    // Deactivate URL-bearing listeners first. This ordering narrows the window
    // in which a Pause/Stop/disconnect can deliver fresh source metadata.
    this.#updated.deactivate();
    this.#activated.deactivate();
    this.#removed.deactivate();
    this.#focusChanged.deactivate();
  }
}
