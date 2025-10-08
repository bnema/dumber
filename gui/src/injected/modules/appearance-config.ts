/**
 * Appearance Configuration Module
 *
 * Handles application appearance configuration including color palettes and fonts.
 * This config is injected by the native layer at startup and made available to all
 * injected modules and components.
 */

export interface ColorPalette {
  background: string;
  surface: string;
  surfaceVariant: string;
  text: string;
  muted: string;
  accent: string;
  border: string;
}

export interface AppearanceConfig {
  theme: "light" | "dark";
  palettes: {
    light: ColorPalette;
    dark: ColorPalette;
  };
  fonts: {
    sans: string;
    serif: string;
    monospace: string;
    defaultSize: number;
  };
}

declare global {
  interface Window {
    __dumber_appearance_config?: AppearanceConfig;
    __dumber_applyPalette?: (theme: "light" | "dark") => void;
  }
}

/**
 * Applies the appearance configuration to the document
 * Sets CSS custom properties for colors and fonts
 */
export function applyAppearanceConfig(): void {
  const config = window.__dumber_appearance_config;
  if (!config) {
    console.warn("[dumber] Appearance config not available");
    return;
  }

  const theme = config.theme || "light";
  const palette = config.palettes[theme];
  const root = document.documentElement;

  // Apply color palette as CSS custom properties
  root.style.setProperty("--color-background", palette.background);
  root.style.setProperty("--color-surface", palette.surface);
  root.style.setProperty("--color-surface-variant", palette.surfaceVariant);
  root.style.setProperty("--color-text", palette.text);
  root.style.setProperty("--color-muted", palette.muted);
  root.style.setProperty("--color-accent", palette.accent);
  root.style.setProperty("--color-border", palette.border);

  // Apply font families
  root.style.setProperty("--font-sans", config.fonts.sans);
  root.style.setProperty("--font-serif", config.fonts.serif);
  root.style.setProperty("--font-mono", config.fonts.monospace);

  console.log(`[dumber] Applied ${theme} appearance config`);

  // Dispatch event for components that need to react to appearance changes
  document.dispatchEvent(
    new CustomEvent("dumber:appearance-applied", {
      detail: { theme, palette, fonts: config.fonts },
    }),
  );
}

/**
 * Gets the current theme from appearance config
 */
export function getCurrentTheme(): "light" | "dark" {
  return window.__dumber_appearance_config?.theme || "light";
}

/**
 * Gets the current color palette for the active theme
 */
export function getCurrentPalette(): ColorPalette | null {
  const config = window.__dumber_appearance_config;
  if (!config) return null;

  return config.palettes[config.theme];
}

/**
 * Expose palette application function globally for integration with color-scheme module
 */
window.__dumber_applyPalette = (theme: "light" | "dark") => {
  const config = window.__dumber_appearance_config;
  if (!config) return;

  const palette = config.palettes[theme];
  const root = document.documentElement;

  root.style.setProperty("--color-background", palette.background);
  root.style.setProperty("--color-surface", palette.surface);
  root.style.setProperty("--color-surface-variant", palette.surfaceVariant);
  root.style.setProperty("--color-text", palette.text);
  root.style.setProperty("--color-muted", palette.muted);
  root.style.setProperty("--color-accent", palette.accent);
  root.style.setProperty("--color-border", palette.border);

  console.log(`[dumber] Switched to ${theme} palette`);
};

// Auto-initialize if config is already available
if (window.__dumber_appearance_config) {
  applyAppearanceConfig();
}

// Listen for config injection and apply
document.addEventListener("DOMContentLoaded", () => {
  if (window.__dumber_appearance_config) {
    applyAppearanceConfig();
  }
});

export default {
  applyAppearanceConfig,
  getCurrentTheme,
  getCurrentPalette,
};
