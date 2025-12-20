<script lang="ts">
  import { onMount } from "svelte";
  import ConfigShell from "./config/ConfigShell.svelte";

  type ColorPalette = {
    background: string;
    surface: string;
    surface_variant: string;
    text: string;
    muted: string;
    accent: string;
    border: string;
  };

  type ConfigDTO = {
    appearance: {
      sans_font: string;
      serif_font: string;
      monospace_font: string;
      default_font_size: number;
      color_scheme: string;
      light_palette: ColorPalette;
      dark_palette: ColorPalette;
    };
    default_ui_scale: number;
    default_search_engine: string;
  };

  const PALETTE_FIELDS: Array<{ key: keyof ColorPalette; label: string }> = [
    { key: "background", label: "Background" },
    { key: "surface", label: "Surface" },
    { key: "surface_variant", label: "Surface Variant" },
    { key: "text", label: "Text" },
    { key: "muted", label: "Muted" },
    { key: "accent", label: "Accent" },
    { key: "border", label: "Border" },
  ];

  function isHexColor(value: string): boolean {
    return /^#[0-9a-fA-F]{6}$/.test(value);
  }


  let config = $state<ConfigDTO | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let saving = $state(false);
  let saveSuccess = $state(false);
  let themeEvents = $state(0);

  async function fetchConfig() {
    try {
      const response = await fetch("/api/config");
      if (!response.ok) throw new Error("Failed to fetch config");
      config = (await response.json()) as ConfigDTO;
    } catch (e: any) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function resetToDefaults() {
    error = null;
    saveSuccess = false;

    const confirmed = window.confirm(
      "Reset this page to default settings? (You still need to click Save to persist.)",
    );
    if (!confirmed) return;

    try {
      const response = await fetch("/api/config/default");
      if (!response.ok) throw new Error("Failed to fetch default config");
      config = (await response.json()) as ConfigDTO;
    } catch (e: any) {
      error = e.message;
    }
  }

  function getWebKitBridge(): { postMessage: (msg: unknown) => void } | null {
    const bridge = (window as any).webkit?.messageHandlers?.dumber;
    if (bridge && typeof bridge.postMessage === "function") {
      return bridge;
    }
    return null;
  }

  function getWebViewId(): number {
    return (window as any).__dumber_webview_id || 0;
  }

  async function saveConfig() {
    saving = true;
    saveSuccess = false;
    error = null;

    try {
      const bridge = getWebKitBridge();
      const webviewId = getWebViewId();

      console.debug("[config] save click", {
        hasBridge: Boolean(bridge),
        webviewId,
        hasConfig: Boolean(config),
      });

      if (!bridge) {
        throw new Error("WebKit bridge not available (not running inside Dumber)");
      }
      if (!webviewId) {
        throw new Error("Missing window.__dumber_webview_id; cannot dispatch/receive callbacks");
      }
      if (!config) {
        throw new Error("Config not loaded");
      }

      let timeout: number | null = window.setTimeout(() => {
        if (saving) {
          console.warn("[config] save timed out (no callback)");
          error = "Save timed out (no response from native handler)";
          saving = false;
        }
      }, 8000);

      const clearSaveTimeout = () => {
        if (timeout != null) {
          window.clearTimeout(timeout);
          timeout = null;
        }
      };

      (window as any).__dumber_config_saved = (resp?: unknown) => {
        clearSaveTimeout();
        console.debug("[config] save success", resp);
        saveSuccess = true;
        saving = false;

        // Refresh config state from backend (in case watcher normalized values)
        fetchConfig();

        setTimeout(() => {
          saveSuccess = false;
        }, 3000);
      };
      (window as any).__dumber_config_error = (msg: unknown) => {
        clearSaveTimeout();
        console.error("[config] save error", msg);
        error = typeof msg === "string" ? msg : "Failed to save config";
        saving = false;
      };

      const payload = $state.snapshot(config);

      try {
        bridge.postMessage({
          type: "save_config",
          webview_id: webviewId,
          payload,
        });
        console.debug("[config] postMessage sent", { payloadBytes: JSON.stringify(payload).length });
      } catch (postErr) {
        console.error("[config] postMessage threw", postErr);
        throw postErr;
      }
    } catch (e: any) {
      console.error("[config] save exception", e);
      error = e.message;
      saving = false;
    }
  }

  onMount(() => {
    console.debug("[config] mount", {
      webviewId: (window as any).__dumber_webview_id,
      hasBridge: Boolean((window as any).webkit?.messageHandlers?.dumber),
    });

    const onThemeChanged = (e: Event) => {
      themeEvents += 1;
      console.debug("[config] theme changed", (e as CustomEvent).detail);
    };
    window.addEventListener('dumber:theme-changed', onThemeChanged);

    fetchConfig();

    return () => {
      window.removeEventListener('dumber:theme-changed', onThemeChanged);
    };
  });
</script>

<ConfigShell>
  <div class="config-content">
    {#if loading}
      <div class="loading">LOADING…</div>
    {:else if error}
      <div class="error">
        <strong>ERROR</strong>
        <span>{error}</span>
      </div>
    {/if}

    {#if config}
      <div class="stack">
      <!-- Appearance Section -->
      <section class="border border-border bg-card p-6 text-card-foreground">
        <h2 class="mb-6 text-xl font-semibold">Appearance</h2>
        
        <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div class="space-y-2">
            <label for="sans_font" class="block text-sm font-medium text-muted-foreground">Sans-Serif Font</label>
            <input
              id="sans_font"
              type="text"
              bind:value={config.appearance.sans_font}
              class="w-full border border-input bg-background px-3 py-2 text-foreground outline-none focus:border-ring"
            />
          </div>

          <div class="space-y-2">
            <label for="serif_font" class="block text-sm font-medium text-muted-foreground">Serif Font</label>
            <input
              id="serif_font"
              type="text"
              bind:value={config.appearance.serif_font}
              class="w-full border border-input bg-background px-3 py-2 text-foreground outline-none focus:border-ring"
            />
          </div>

          <div class="space-y-2">
            <label for="monospace_font" class="block text-sm font-medium text-muted-foreground">Monospace Font</label>
            <input
              id="monospace_font"
              type="text"
              bind:value={config.appearance.monospace_font}
              class="w-full border border-input bg-background px-3 py-2 text-foreground outline-none focus:border-ring"
            />
          </div>

          <div class="space-y-2">
            <label for="font_size" class="block text-sm font-medium text-muted-foreground">Default Font Size</label>
            <input
              id="font_size"
              type="number"
              bind:value={config.appearance.default_font_size}
              class="w-full border border-input bg-background px-3 py-2 text-foreground outline-none focus:border-ring"
            />
          </div>

          <div class="space-y-2">
            <label for="color_scheme" class="block text-sm font-medium text-muted-foreground">Color Scheme</label>
            <select
              id="color_scheme"
              bind:value={config.appearance.color_scheme}
              class="w-full border border-input bg-background px-3 py-2 text-foreground outline-none focus:border-ring"
            >
              <option value="default">Follow System</option>
              <option value="prefer-dark">Always Dark</option>
              <option value="prefer-light">Always Light</option>
            </select>
          </div>

          <div class="space-y-2">
            <label for="ui_scale" class="block text-sm font-medium text-muted-foreground">UI Scale</label>
            <input
              id="ui_scale"
              type="number"
              step="0.1"
              bind:value={config.default_ui_scale}
              class="w-full border border-input bg-background px-3 py-2 text-foreground outline-none focus:border-ring"
            />
          </div>
        </div>

        <div class="mt-8 space-y-8">
          <div>
            <h3 class="mb-3 text-base font-semibold">Theme: Light</h3>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              {#each PALETTE_FIELDS as field (field.key)}
                <div class="space-y-2">
                  <label class="block text-sm font-medium text-muted-foreground" for={`light_${field.key}`}
                    >{field.label}</label
                  >
                  <div class="flex items-center gap-3">
                    <input
                      id={`light_${field.key}`}
                      type="color"
                      value={config.appearance.light_palette[field.key]}
                      oninput={(e) => {
                        const v = (e.currentTarget as HTMLInputElement).value;
                        config!.appearance.light_palette[field.key] = v;
                      }}
                      class="h-10 w-12 border border-input bg-background"
                    />
                    <input
                      type="text"
                      bind:value={config.appearance.light_palette[field.key]}
                      class="w-full border border-input bg-background px-3 py-2 text-foreground outline-none focus:border-ring"
                    />
                  </div>
                  {#if !isHexColor(config.appearance.light_palette[field.key])}
                    <p class="text-xs text-muted-foreground">Expected hex color like #RRGGBB</p>
                  {/if}
                </div>
              {/each}
            </div>
          </div>

          <div>
            <h3 class="mb-3 text-base font-semibold">Theme: Dark</h3>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              {#each PALETTE_FIELDS as field (field.key)}
                <div class="space-y-2">
                  <label class="block text-sm font-medium text-muted-foreground" for={`dark_${field.key}`}
                    >{field.label}</label
                  >
                  <div class="flex items-center gap-3">
                    <input
                      id={`dark_${field.key}`}
                      type="color"
                      value={config.appearance.dark_palette[field.key]}
                      oninput={(e) => {
                        const v = (e.currentTarget as HTMLInputElement).value;
                        config!.appearance.dark_palette[field.key] = v;
                      }}
                      class="h-10 w-12 border border-input bg-background"
                    />
                    <input
                      type="text"
                      bind:value={config.appearance.dark_palette[field.key]}
                      class="w-full border border-input bg-background px-3 py-2 text-foreground outline-none focus:border-ring"
                    />
                  </div>
                  {#if !isHexColor(config.appearance.dark_palette[field.key])}
                    <p class="text-xs text-muted-foreground">Expected hex color like #RRGGBB</p>
                  {/if}
                </div>
              {/each}
            </div>
          </div>
        </div>
      </section>

      <!-- Search Section -->
      <section class="border border-border bg-card p-6 text-card-foreground">
        <h2 class="mb-6 text-xl font-semibold">Search</h2>
        <div class="space-y-4">
          <div class="space-y-2">
            <label for="search_engine" class="block text-sm font-medium text-muted-foreground">Default Search Engine (use %s for query)</label>
            <input
              id="search_engine"
              type="text"
              bind:value={config.default_search_engine}
              class="w-full border border-input bg-background px-3 py-2 text-foreground outline-none focus:border-ring"
            />
          </div>
        </div>
      </section>

      <div class="actions">
        {#if saveSuccess}
          <span class="saved">SAVED</span>
        {/if}
        {#if themeEvents > 0}
          <span class="hint">THEME×{themeEvents}</span>
        {/if}
        <button onclick={resetToDefaults} disabled={saving} class="reset-button" type="button">
          RESET DEFAULTS
        </button>
        <button onclick={saveConfig} disabled={saving} class="save-button" type="button">
          {saving ? "SAVING…" : "SAVE"}
        </button>
      </div>
    </div>
  {/if}
</div>
</ConfigShell>

<style>
  .config-content {
    width: 100%;
    max-width: 980px;
    margin: 0 auto;
    padding: 1.5rem 1.25rem 2rem;
  }

  .stack {
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }

  .loading {
    padding: 2rem 0;
    color: var(--muted-foreground);
    letter-spacing: 0.1em;
    font-size: 0.8rem;
  }

  .error {
    padding: 0.75rem 1rem;
    border: 1px solid color-mix(in srgb, var(--border) 50%, #ef4444 50%);
    background: color-mix(in srgb, var(--card) 70%, #ef4444 30%);
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.75rem;
    padding-top: 0.75rem;
    border-top: 1px solid var(--border);
  }

  .saved {
    font-size: 0.7rem;
    letter-spacing: 0.12em;
    color: var(--primary, #4ade80);
  }

  .hint {
    font-size: 0.7rem;
    letter-spacing: 0.12em;
    color: var(--muted-foreground);
  }

  .reset-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0.55rem 1rem;
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.12em;
    color: var(--foreground);
    background: transparent;
    border: 1px solid var(--border);
    cursor: pointer;
    transition: all 120ms ease;
  }

  .reset-button:hover {
    background: color-mix(in srgb, var(--card) 25%, transparent);
  }

  .reset-button:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .save-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0.55rem 1rem;
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.12em;
    color: var(--background);
    background: var(--primary, #4ade80);
    border: 1px solid color-mix(in srgb, var(--primary, #4ade80) 70%, var(--border) 30%);
    cursor: pointer;
    transition: all 120ms ease;
  }

  .save-button:hover {
    filter: brightness(1.02);
  }

  .save-button:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
</style>

