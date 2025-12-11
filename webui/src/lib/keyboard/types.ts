/**
 * Keyboard Service Types
 *
 * Type definitions for the unified keyboard shortcut system.
 * Provides type-safe interfaces for registering and handling keyboard shortcuts.
 */

export interface ShortcutConfig {
  /** The keyboard shortcut key combination (e.g., 'ctrl+l', 'escape', 'enter') */
  key: string;

  /** Function to execute when the shortcut is triggered */
  handler: () => void;

  /** Whether to prevent the default browser behavior (default: true) */
  preventDefault?: boolean;

  /** Whether to stop event propagation (default: true) */
  stopPropagation?: boolean;

  /** Only trigger when this component is focused (default: false) */
  whenFocused?: boolean;

  /** Optional description for debugging */
  description?: string;
}

export interface KeyboardEventDetail {
  /** The original keyboard shortcut string */
  shortcut: string;

  /** Timestamp when the event occurred */
  timestamp: number;

  /** Component that should handle this event (if specified) */
  targetComponent?: string;
}

export interface ComponentShortcuts {
  /** Unique component identifier */
  componentId: string;

  /** Array of shortcuts registered by this component */
  shortcuts: ShortcutConfig[];

  /** Whether this component is currently focused */
  focused: boolean;

  /** Optional cleanup function called when component is destroyed */
  cleanup?: () => void;
}

export interface KeyboardServiceConfig {
  /** Enable debug logging for keyboard events */
  debug?: boolean;

  /** Global shortcuts that work regardless of component focus */
  globalShortcuts?: ShortcutConfig[];
}

/**
 * Event types for keyboard service
 */
export type KeyboardEventType =
  | "shortcut-triggered"
  | "component-focused"
  | "component-blurred"
  | "shortcut-registered"
  | "shortcut-unregistered";

export interface KeyboardEvent {
  type: KeyboardEventType;
  detail: KeyboardEventDetail | ComponentShortcuts | ShortcutConfig;
}

/**
 * Keyboard modifiers mapping
 */
export const MODIFIER_KEYS = {
  ctrl: "ctrlKey",
  alt: "altKey",
  shift: "shiftKey",
} as const;

/**
 * Common shortcut patterns used throughout the application
 */
export const COMMON_SHORTCUTS = {
  OMNIBOX_OPEN: "ctrl+l",
  FIND_OPEN: "ctrl+f",
  ESCAPE: "escape",
  ENTER: "enter",
  ARROW_UP: "arrowup",
  ARROW_DOWN: "arrowdown",
  ARROW_LEFT: "arrowleft",
  ARROW_RIGHT: "arrowright",
  TAB: "tab",
  SHIFT_TAB: "shift+tab",
} as const;
