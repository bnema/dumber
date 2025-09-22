import type { HistoryEntry, SearchShortcut } from "./generated.js";

export interface AppState {
  currentURL: string;
  currentZoom: number;
  history: HistoryEntry[];
  shortcuts: Record<string, SearchShortcut>;
  isLoading: boolean;
}

export interface NotificationOptions {
  message: string;
  type?: "info" | "success" | "error";
  duration?: number;
}

export interface KeyboardShortcut {
  key: string;
  ctrlKey?: boolean;
  metaKey?: boolean;
  shiftKey?: boolean;
  altKey?: boolean;
  handler: () => void | Promise<void>;
}

export { HistoryEntry, SearchShortcut } from "./generated.js";
