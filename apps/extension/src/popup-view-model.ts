import type { SessionState } from "./capture-authority.js";

export type PopupAction = "start" | "pause" | "resume" | "stop";

export interface PopupViewModel {
  heading: string;
  detail: string;
  actions: readonly PopupAction[];
  recording: boolean;
}

export function popupViewForState(state: SessionState): PopupViewModel {
  switch (state) {
    case "IDLE":
      return view("Not recording", "Start a session when you want CurioTrace to observe allowed browsing.", ["start"], false);
    case "RECORDING":
      return view("Recording", "Allowed browsing content may be observed until you pause or stop.", ["pause", "stop"], true);
    case "PAUSED":
      return view("Paused", "Browsing content is not being captured. Resume this session or stop it.", ["resume", "stop"], false);
    case "INTERRUPTED":
      return view("Interrupted", "Recording did not resume automatically. Choose Resume or Stop explicitly.", ["resume", "stop"], false);
    case "FINISHED":
      return view("Session finished", "Start when you want to begin a new browsing session.", ["start"], false);
  }
}

function view(
  heading: string,
  detail: string,
  actions: readonly PopupAction[],
  recording: boolean,
): PopupViewModel {
  return Object.freeze({ heading, detail, actions: Object.freeze([...actions]), recording });
}
