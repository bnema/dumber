/**
 * Keyboard Service Module
 *
 * Centralized keyboard shortcut management for Dumber Browser.
 * Provides type-safe keyboard event handling and component integration.
 */

export { KeyboardService, keyboardService } from './KeyboardService';
export type {
  ShortcutConfig,
  ComponentShortcuts,
  KeyboardServiceConfig,
  KeyboardEventDetail,
  KeyboardEvent,
  KeyboardEventType
} from './types';
export { MODIFIER_KEYS, COMMON_SHORTCUTS } from './types';