/**
 * Color Scheme Detection Module
 *
 * Handles GTK theme detection and applies appropriate color scheme preferences
 * to web pages. This module is injected at document-start to ensure proper
 * theme initialization before page content loads.
 */

declare global {
  interface Window {
    __dumber_gtk_prefers_dark?: boolean;
    __dumber_setTheme?: (theme: 'light' | 'dark') => void;
  }
}

interface ColorSchemeConfig {
  isDark: boolean;
  colorScheme: 'light' | 'dark';
  metaContent: string;
  rootStyle: string;
}

class ColorSchemeManager {
  private config: ColorSchemeConfig;

  constructor() {
    // Use GTK preference if available, otherwise detect from system
    const prefersDark = window.__dumber_gtk_prefers_dark ?? false;

    this.config = {
      isDark: prefersDark,
      colorScheme: prefersDark ? 'dark' : 'light',
      metaContent: prefersDark ? 'dark light' : 'light dark',
      rootStyle: prefersDark ? ':root{color-scheme:dark;}' : ':root{color-scheme:light;}'
    };

    this.initialize();
  }

  private initialize(): void {
    try {
      console.log(`[dumber] color-scheme set: ${this.config.colorScheme}`);
      console.log(`[dumber] GTK detected color mode: ${this.config.colorScheme}`);

      // Notify native layer of theme preference
      this.notifyNativeLayer();

      // Inject color-scheme meta tag
      this.injectColorSchemeMeta();

      // Apply root styles
      this.applyRootStyles();

      // Override matchMedia for proper prefers-color-scheme support
      this.overrideMatchMedia();

      // Set up theme integration with existing UI components
      this.setupThemeIntegration();

    } catch (error) {
      console.warn('[dumber] color-scheme injection failed', error);
    }
  }

  private notifyNativeLayer(): void {
    try {
      (window as any).webkit?.messageHandlers?.dumber?.postMessage(
        JSON.stringify({
          type: 'theme',
          value: this.config.colorScheme
        })
      );
    } catch {
      // Ignore errors - native layer may not be available
    }
  }

  private injectColorSchemeMeta(): void {
    const meta = document.createElement('meta');
    meta.name = 'color-scheme';
    meta.content = this.config.metaContent;
    document.documentElement.appendChild(meta);
  }

  private applyRootStyles(): void {
    const style = document.createElement('style');
    style.textContent = this.config.rootStyle;
    document.documentElement.appendChild(style);
  }

  private overrideMatchMedia(): void {
    const darkQuery = '(prefers-color-scheme: dark)';
    const lightQuery = '(prefers-color-scheme: light)';
    const originalMatchMedia = window.matchMedia;

    window.matchMedia = (query: string): MediaQueryList => {
      if (typeof query === 'string' && (query.includes(darkQuery) || query.includes(lightQuery))) {
        // Create a mock MediaQueryList that reflects our GTK preference
        const matches = query.includes('dark') ? this.config.isDark : !this.config.isDark;

        return {
          matches,
          media: query,
          onchange: null,
          addListener: () => {},
          removeListener: () => {},
          addEventListener: () => {},
          removeEventListener: () => {},
          dispatchEvent: () => false
        } as MediaQueryList;
      }

      // For non-color-scheme queries, use the original implementation
      return originalMatchMedia.call(window, query);
    };
  }

  private setupThemeIntegration(): void {
    // Integrate with unified theme setter if available
    if (window.__dumber_setTheme) {
      window.__dumber_setTheme(this.config.colorScheme);
    } else {
      // Retry after a short delay in case the bridge hasn't loaded yet
      setTimeout(() => {
        if (window.__dumber_setTheme) {
          window.__dumber_setTheme(this.config.colorScheme);
        } else {
          // Fallback: directly apply dark class for Tailwind compatibility
          console.warn('[dumber] Theme setter not available, using fallback');
          if (this.config.isDark) {
            document.documentElement.classList.add('dark');
          } else {
            document.documentElement.classList.remove('dark');
          }
        }
      }, 100);
    }
  }

  /**
   * Updates the color scheme preference at runtime
   * @param isDark Whether to use dark mode
   */
  public updateColorScheme(isDark: boolean): void {
    this.config.isDark = isDark;
    this.config.colorScheme = isDark ? 'dark' : 'light';
    this.config.metaContent = isDark ? 'dark light' : 'light dark';
    this.config.rootStyle = isDark ? ':root{color-scheme:dark;}' : ':root{color-scheme:light;}';

    // Re-apply all configurations
    this.initialize();
  }
}

// Initialize color scheme management
const colorSchemeManager = new ColorSchemeManager();

// Export for potential runtime updates
(window as any).__dumber_color_scheme_manager = colorSchemeManager;

export default colorSchemeManager;