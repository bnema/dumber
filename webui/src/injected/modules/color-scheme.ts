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
    __dumber_setTheme?: (theme: "light" | "dark") => void;
  }
}

interface ColorSchemeConfig {
  isDark: boolean;
  colorScheme: "light" | "dark";
  metaContent: string;
  rootStyle: string;
}

class ColorSchemeManager {
  private config: ColorSchemeConfig;
  private metaElement: HTMLMetaElement | null = null;
  private styleElement: HTMLStyleElement | null = null;
  private userPreference: "light" | "dark" | null = null;
  private static readonly STORAGE_KEY = "dumber.theme";
  private static instance: ColorSchemeManager | null = null;
  private static matchMediaPatched = false;
  private static originalMatchMedia: typeof window.matchMedia | null = null;

  constructor() {
    ColorSchemeManager.instance = this;

    this.userPreference = this.getStoredPreference();

    // Use GTK preference if available, otherwise detect from system
    const prefersDark = this.userPreference
      ? this.userPreference === "dark"
      : (window.__dumber_gtk_prefers_dark ?? false);

    this.config = {
      isDark: prefersDark,
      colorScheme: prefersDark ? "dark" : "light",
      metaContent: prefersDark ? "dark light" : "light dark",
      rootStyle: prefersDark
        ? ":root{color-scheme:dark;}"
        : ":root{color-scheme:light;}",
    };

    this.initialize();
  }

  private initialize(): void {
    try {
      console.log(`[dumber] color-scheme set: ${this.config.colorScheme}`);
      console.log(
        `[dumber] GTK detected color mode: ${this.config.colorScheme}`,
      );

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
      console.warn("[dumber] color-scheme injection failed", error);
    }
  }

  private notifyNativeLayer(): void {
    try {
      (window as any).webkit?.messageHandlers?.dumber?.postMessage({
        type: "theme",
        payload: { value: this.config.colorScheme },
      });
    } catch {
      // Ignore errors - native layer may not be available
    }
  }

  private injectColorSchemeMeta(): void {
    if (this.metaElement && this.metaElement.isConnected) {
      this.metaElement.content = this.config.metaContent;
      return;
    }

    const existing = document.querySelector(
      'meta[name="color-scheme"]',
    ) as HTMLMetaElement | null;
    const meta = existing ?? document.createElement("meta");
    meta.name = "color-scheme";
    meta.content = this.config.metaContent;
    if (!existing) {
      document.documentElement.appendChild(meta);
    }
    this.metaElement = meta;
  }

  private applyRootStyles(): void {
    if (this.styleElement && this.styleElement.isConnected) {
      this.styleElement.textContent = this.config.rootStyle;
      return;
    }

    const existing = document.querySelector(
      "style[data-dumber-color-scheme]",
    ) as HTMLStyleElement | null;
    const style = existing ?? document.createElement("style");
    style.setAttribute("data-dumber-color-scheme", "");
    style.textContent = this.config.rootStyle;
    if (!existing) {
      document.documentElement.appendChild(style);
    }
    this.styleElement = style;
  }

  private overrideMatchMedia(): void {
    if (ColorSchemeManager.matchMediaPatched) {
      return;
    }

    const darkQuery = "(prefers-color-scheme: dark)";
    const lightQuery = "(prefers-color-scheme: light)";
    ColorSchemeManager.originalMatchMedia = window.matchMedia.bind(window);

    window.matchMedia = (query: string): MediaQueryList => {
      const manager = ColorSchemeManager.instance;
      if (
        manager &&
        typeof query === "string" &&
        (query.includes(darkQuery) || query.includes(lightQuery))
      ) {
        const matches = query.includes("dark")
          ? manager.config.isDark
          : !manager.config.isDark;

        return {
          matches,
          media: query,
          onchange: null,
          addListener: () => {},
          removeListener: () => {},
          addEventListener: () => {},
          removeEventListener: () => {},
          dispatchEvent: () => false,
        } as MediaQueryList;
      }

      const original = ColorSchemeManager.originalMatchMedia;
      if (!original) {
        throw new Error("Original matchMedia not available");
      }

      return original.call(window, query);
    };

    ColorSchemeManager.matchMediaPatched = true;
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
          console.warn("[dumber] Theme setter not available, using fallback");
          if (this.config.isDark) {
            document.documentElement.classList.add("dark");
          } else {
            document.documentElement.classList.remove("dark");
          }
          try {
            window.__dumber_applyPalette?.(
              this.config.isDark ? "dark" : "light",
            );
          } catch {
            /* noop */
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
    this.config.colorScheme = isDark ? "dark" : "light";
    this.config.metaContent = isDark ? "dark light" : "light dark";
    this.config.rootStyle = isDark
      ? ":root{color-scheme:dark;}"
      : ":root{color-scheme:light;}";

    // Re-apply all configurations
    this.initialize();
  }

  private getStoredPreference(): "light" | "dark" | null {
    try {
      const value = localStorage.getItem(ColorSchemeManager.STORAGE_KEY);
      if (value === "light" || value === "dark") {
        return value;
      }
    } catch (error) {
      console.warn("[dumber] Failed to read stored theme preference", error);
    }
    return null;
  }

  private storePreference(theme: "light" | "dark"): void {
    try {
      localStorage.setItem(ColorSchemeManager.STORAGE_KEY, theme);
    } catch (error) {
      console.warn("[dumber] Failed to persist theme preference", error);
    }
  }

  /**
   * Persist and apply an explicit user preference
   */
  public setUserPreference(theme: "light" | "dark"): void {
    if (this.userPreference === theme && this.config.colorScheme === theme) {
      return;
    }
    this.userPreference = theme;
    this.storePreference(theme);
    this.updateColorScheme(theme === "dark");
  }

  public getCurrentTheme(): "light" | "dark" {
    return this.config.colorScheme;
  }
}

// Initialize color scheme management
const colorSchemeManager = new ColorSchemeManager();

// Export for potential runtime updates
(window as any).__dumber_color_scheme_manager = colorSchemeManager;

export default colorSchemeManager;

declare global {
  interface Window {
    __dumber_color_scheme_manager?: {
      setUserPreference: (theme: "light" | "dark") => void;
      getCurrentTheme: () => "light" | "dark";
    };
  }
}
