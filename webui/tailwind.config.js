/** @type {import('tailwindcss').Config} */
export default {
  content: ["./src/**/*.{html,js,ts,svelte}"],
  theme: {
    extend: {
      // Font families for Industrial Terminal aesthetic
      fontFamily: {
        mono: ['"JetBrains Mono NF"', "ui-monospace", "SFMono-Regular", "monospace"],
        sans: ["Inter", "system-ui", "-apple-system", "sans-serif"],
      },
      // Custom design system colors
      colors: {
        border: "var(--border)",
        input: "var(--input)",
        ring: "var(--ring)",
        background: "var(--background)",
        foreground: "var(--foreground)",
        primary: "var(--primary)",
        "primary-foreground": "var(--primary-foreground)",
        secondary: "var(--secondary)",
        "secondary-foreground": "var(--secondary-foreground)",
        destructive: "var(--destructive)",
        "destructive-foreground": "var(--destructive-foreground)",
        muted: "var(--muted)",
        "muted-foreground": "var(--muted-foreground)",
        accent: "var(--accent)",
        "accent-foreground": "var(--accent-foreground)",
        popover: "var(--popover)",
        "popover-foreground": "var(--popover-foreground)",
        card: "var(--card)",
        "card-foreground": "var(--card-foreground)",
        // Browser-specific colors that work well in both themes
        browser: {
          bg: "rgb(var(--browser-bg) / <alpha-value>)",
          surface: "rgb(var(--browser-surface) / <alpha-value>)",
          text: "rgb(var(--browser-text) / <alpha-value>)",
          muted: "rgb(var(--browser-muted) / <alpha-value>)",
          accent: "rgb(var(--browser-accent) / <alpha-value>)",
          border: "rgb(var(--browser-border) / <alpha-value>)",
        },
      },
      // Animation speeds optimized for browser UI
      transitionDuration: {
        150: "150ms", // Fast interactions
        250: "250ms", // UI feedback
        400: "400ms", // Smooth transitions
      },
      // Typography scale for browser UI
      fontSize: {
        xs: ["0.75rem", "1rem"],
        sm: ["0.875rem", "1.25rem"],
        base: ["1rem", "1.5rem"],
        lg: ["1.125rem", "1.75rem"],
      },
      // Spacing for compact UI elements
      spacing: {
        18: "4.5rem",
        22: "5.5rem",
      },
    },
  },
  plugins: [],
  // Manual dark mode control (not system preference)
  // This allows GTK to control theme via CSS class
  darkMode: "class",
};
