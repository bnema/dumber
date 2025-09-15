/**
 * Keyboard Service
 *
 * Centralized keyboard shortcut management for the Dumber Browser GUI.
 * Bridges between native GTK keyboard events and Svelte component handlers.
 */

import type {
  ShortcutConfig,
  ComponentShortcuts,
  KeyboardServiceConfig,
  KeyboardEventDetail,
  KeyboardEvent,
  KeyboardEventType
} from './types';
import { COMMON_SHORTCUTS } from './types';

export class KeyboardService {
  private components = new Map<string, ComponentShortcuts>();
  private activeComponent: string | null = null;
  private globalShortcuts: ShortcutConfig[] = [];
  private eventListeners = new Map<KeyboardEventType, Set<(event: KeyboardEvent) => void>>();
  private debug: boolean = false;

  constructor(config: KeyboardServiceConfig = {}) {
    this.debug = config.debug ?? false;
    this.globalShortcuts = config.globalShortcuts ?? [];

    this.log('info', 'KeyboardService initialized', { config });
  }

  /**
   * Register shortcuts for a component
   */
  registerShortcuts(componentId: string, shortcuts: ShortcutConfig[]): void {
    if (this.components.has(componentId)) {
      this.log('warn', `Component ${componentId} already registered, overwriting shortcuts`);
    }

    const componentShortcuts: ComponentShortcuts = {
      componentId,
      shortcuts: shortcuts.map(shortcut => ({
        ...shortcut,
        preventDefault: shortcut.preventDefault ?? true,
        stopPropagation: shortcut.stopPropagation ?? true,
        whenFocused: shortcut.whenFocused ?? false
      })),
      focused: false
    };

    this.components.set(componentId, componentShortcuts);

    this.log('info', 'Shortcuts registered', { componentId, shortcuts: shortcuts.length });
    this.emit('shortcut-registered', {
      shortcut: componentId,
      timestamp: Date.now(),
      targetComponent: componentId
    });
  }

  /**
   * Unregister shortcuts for a component
   */
  unregisterShortcuts(componentId: string): void {
    const component = this.components.get(componentId);
    if (!component) {
      this.log('warn', `Component ${componentId} not found for unregistration`);
      return;
    }

    // Call cleanup function if provided
    if (component.cleanup) {
      component.cleanup();
    }

    this.components.delete(componentId);

    // Clear active component if it was this one
    if (this.activeComponent === componentId) {
      this.activeComponent = null;
    }

    this.log('info', 'Shortcuts unregistered', { componentId });
    this.emit('shortcut-unregistered', {
      shortcut: componentId,
      timestamp: Date.now(),
      targetComponent: componentId
    });
  }

  /**
   * Set which component is currently focused
   */
  setActiveComponent(componentId: string | null): void {
    // Blur previous component
    if (this.activeComponent && this.components.has(this.activeComponent)) {
      const prevComponent = this.components.get(this.activeComponent)!;
      prevComponent.focused = false;
      this.emit('component-blurred', {
        shortcut: this.activeComponent,
        timestamp: Date.now(),
        targetComponent: this.activeComponent
      });
    }

    this.activeComponent = componentId;

    // Focus new component
    if (componentId && this.components.has(componentId)) {
      const newComponent = this.components.get(componentId)!;
      newComponent.focused = true;
      this.emit('component-focused', {
        shortcut: componentId,
        timestamp: Date.now(),
        targetComponent: componentId
      });
    }

    this.log('info', 'Active component changed', { from: this.activeComponent, to: componentId });
  }

  /**
   * Handle a keyboard shortcut from native GTK layer
   */
  handleNativeShortcut(shortcut: string): boolean {
    this.log('info', 'Native shortcut received', { shortcut });

    // Normalize shortcut string
    const normalizedShortcut = this.normalizeShortcut(shortcut);

    // Try to handle with focused component first
    if (this.activeComponent) {
      const handled = this.handleComponentShortcut(this.activeComponent, normalizedShortcut, true);
      if (handled) {
        return true;
      }
    }

    // Try global shortcuts
    const globalHandled = this.handleGlobalShortcuts(normalizedShortcut);
    if (globalHandled) {
      return true;
    }

    // Try all components (for non-focused shortcuts)
    for (const [componentId] of this.components) {
      if (componentId === this.activeComponent) continue; // Already tried above

      const handled = this.handleComponentShortcut(componentId, normalizedShortcut, false);
      if (handled) {
        return true;
      }
    }

    this.log('info', 'Shortcut not handled', { shortcut: normalizedShortcut });
    return false;
  }

  /**
   * Handle JavaScript keyboard events (for web page events)
   */
  handleKeyboardEvent(event: globalThis.KeyboardEvent): boolean {
    const shortcut = this.eventToShortcut(event);
    if (!shortcut) return false;

    this.log('info', 'Keyboard event received', { shortcut });
    return this.handleNativeShortcut(shortcut);
  }

  /**
   * Handle JavaScript mouse events (for navigation buttons)
   */
  handleMouseEvent(event: globalThis.MouseEvent): boolean {
    try {
      // Mouse button 3 (back button)
      if (event.button === 3) {
        event.preventDefault();
        event.stopPropagation();

        if (window.history.length > 1) {
          console.log('⬅️ Mouse back button pressed');
          window.history.back();
        }
        return true;
      }

      // Mouse button 4 (forward button)
      if (event.button === 4) {
        event.preventDefault();
        event.stopPropagation();

        console.log('➡️ Mouse forward button pressed');
        window.history.forward();
        return true;
      }

    } catch (error) {
      this.log('error', 'Mouse handler error', { error });
    }

    return false;
  }

  /**
   * Add event listener for keyboard service events
   */
  addEventListener(type: KeyboardEventType, listener: (event: KeyboardEvent) => void): void {
    if (!this.eventListeners.has(type)) {
      this.eventListeners.set(type, new Set());
    }
    this.eventListeners.get(type)!.add(listener);
  }

  /**
   * Remove event listener
   */
  removeEventListener(type: KeyboardEventType, listener: (event: KeyboardEvent) => void): void {
    const listeners = this.eventListeners.get(type);
    if (listeners) {
      listeners.delete(listener);
    }
  }

  /**
   * Get all registered shortcuts for debugging
   */
  getDebugInfo(): object {
    return {
      components: Array.from(this.components.entries()).map(([id, comp]) => ({
        id,
        shortcuts: comp.shortcuts.length,
        focused: comp.focused
      })),
      activeComponent: this.activeComponent,
      globalShortcuts: this.globalShortcuts.length
    };
  }

  // Private methods

  private handleComponentShortcut(componentId: string, shortcut: string, respectFocus: boolean): boolean {
    const component = this.components.get(componentId);
    if (!component) return false;

    for (const shortcutConfig of component.shortcuts) {
      if (this.normalizeShortcut(shortcutConfig.key) === shortcut) {
        // Check focus requirement
        if (respectFocus && shortcutConfig.whenFocused && !component.focused) {
          continue;
        }

        try {
          shortcutConfig.handler();
          this.log('info', 'Shortcut handled', {
            shortcut,
            componentId,
            focused: component.focused,
            description: shortcutConfig.description
          });

          this.emit('shortcut-triggered', {
            shortcut,
            timestamp: Date.now(),
            targetComponent: componentId
          });

          return true;
        } catch (error) {
          this.log('error', 'Shortcut handler error', { shortcut, componentId, error });
        }
      }
    }

    return false;
  }

  private handleGlobalShortcuts(shortcut: string): boolean {
    for (const shortcutConfig of this.globalShortcuts) {
      if (this.normalizeShortcut(shortcutConfig.key) === shortcut) {
        try {
          shortcutConfig.handler();
          this.log('info', 'Global shortcut handled', { shortcut, description: shortcutConfig.description });

          this.emit('shortcut-triggered', {
            shortcut,
            timestamp: Date.now()
          });

          return true;
        } catch (error) {
          this.log('error', 'Global shortcut handler error', { shortcut, error });
        }
      }
    }
    return false;
  }

  private normalizeShortcut(shortcut: string): string {
    return shortcut.toLowerCase().replace(/\s+/g, '');
  }

  private eventToShortcut(event: globalThis.KeyboardEvent): string | null {
    const parts: string[] = [];

    // Add modifiers using MODIFIER_KEYS mapping
    if (event.ctrlKey && event.metaKey) {
      // Both ctrl and meta pressed - use cmdorctrl
      parts.push('cmdorctrl');
    } else if (event.ctrlKey) {
      parts.push('ctrl');
    } else if (event.metaKey) {
      parts.push('cmd');
    }

    if (event.altKey) parts.push('alt');
    if (event.shiftKey) parts.push('shift');

    // Add key
    if (event.key) {
      parts.push(event.key.toLowerCase());
    }

    return parts.length > 0 ? parts.join('+') : null;
  }

  private emit(type: KeyboardEventType, detail: KeyboardEventDetail): void {
    const listeners = this.eventListeners.get(type);
    if (listeners) {
      const keyboardEvent: KeyboardEvent = { type, detail };
      listeners.forEach(listener => {
        try {
          listener(keyboardEvent);
        } catch (error) {
          this.log('error', 'Event listener error', { type, error });
        }
      });
    }
  }

  private log(level: 'info' | 'warn' | 'error' | string, message?: string, data?: unknown): void {
    if (!this.debug && level !== 'error') return;

    const logMessage = message ? `[KeyboardService] ${message}` : `[KeyboardService] ${level}`;

    switch (level) {
      case 'error':
        console.error(logMessage, data);
        break;
      case 'warn':
        console.warn(logMessage, data);
        break;
      default:
        console.log(logMessage, data);
    }
  }
}

// Create and export singleton instance with global shortcuts
export const keyboardService = new KeyboardService({
  debug: false, // Disable debug logging in production
  globalShortcuts: [
    // Navigation shortcuts (migrated from controls module)
    {
      key: 'alt+arrowleft',
      handler: () => {
        if (window.history.length > 1) {
          window.history.back();
        }
      },
      description: 'Navigate back (Alt+Left Arrow)'
    },
    {
      key: 'alt+arrowright',
      handler: () => {
        window.history.forward();
      },
      description: 'Navigate forward (Alt+Right Arrow)'
    },
    // Global UI shortcuts (migrated from UCM JavaScript)
    {
      key: COMMON_SHORTCUTS.OMNIBOX_OPEN,
      handler: () => {
        try {
          if (window.__dumber_omnibox?.toggle) {
            console.log('🎯 Using omnibox API directly');
            window.__dumber_omnibox.toggle();
          } else if (typeof window.__dumber_toggle === 'function') {
            console.log('🎯 Using legacy toggle function');
            window.__dumber_toggle();
          } else {
            console.warn('⚠️ No omnibox toggle function available');
          }
        } catch (error) {
          console.error('❌ Error in omnibox open handler:', error);
        }
      },
      description: 'Open omnibox (Ctrl/Cmd+L)'
    },
    {
      key: COMMON_SHORTCUTS.FIND_OPEN,
      handler: () => {
        try {
          if (window.__dumber_omnibox?.open) {
            console.log('🔍 Using omnibox find API directly');
            window.__dumber_omnibox.open('find');
          } else if (typeof window.__dumber_find_open === 'function') {
            console.log('🔍 Using legacy find function');
            window.__dumber_find_open();
          } else {
            console.warn('⚠️ No find function available');
          }
        } catch (error) {
          console.error('❌ Error in find open handler:', error);
        }
      },
      description: 'Open find bar (Ctrl/Cmd+F)'
    },
    // Copy URL shortcut (Ctrl/Cmd+Shift+C)
    {
      key: 'cmdorctrl+shift+c',
      handler: async () => {
        try {
          const url = window.location.href;
          await navigator.clipboard.writeText(url);

          // Show toast notification
          if (typeof window.__dumber_showToast === 'function') {
            window.__dumber_showToast('URL copied to clipboard!', 2000, 'success');
          }
        } catch (error) {
          console.error('Failed to copy URL:', error);

          // Show error toast
          if (typeof window.__dumber_showToast === 'function') {
            window.__dumber_showToast('Failed to copy URL', 2000, 'error');
          }
        }
      },
      description: 'Copy current URL to clipboard (Ctrl/Cmd+Shift+C)'
    }
  ]
});

// Export types for use in components
export type { ShortcutConfig, ComponentShortcuts, KeyboardServiceConfig } from './types';
export { COMMON_SHORTCUTS } from './types';