<script lang="ts">
  import { onMount } from "svelte";
  import ConfigShell from "./config/ConfigShell.svelte";
  import ShortcutsTable from "./config/ShortcutsTable.svelte";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import { Button } from "$lib/components/ui/button";
  import { ColorPicker } from "$lib/components/ui/color-picker";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { Spinner } from "$lib/components/ui/spinner";
  import * as Card from "$lib/components/ui/card";
  import * as Tabs from "$lib/components/ui/tabs";

  type ColorPalette = {
    background: string;
    surface: string;
    surface_variant: string;
    text: string;
    muted: string;
    accent: string;
    border: string;
  };

  type SearchShortcut = {
    url: string;
    description: string;
  };

  type ResolvedPerformance = {
    skia_cpu_threads: number;
    skia_gpu_threads: number;
    web_process_memory_mb: number;
    network_process_memory_mb: number;
    webview_pool_prewarm: number;
    conservative_threshold: number;
    strict_threshold: number;
    kill_threshold: number;
  };

  type PerformanceConfig = {
    profile: string;
    resolved: ResolvedPerformance;
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
    performance: PerformanceConfig;
    default_ui_scale: number;
    default_search_engine: string;
    search_shortcuts: Record<string, SearchShortcut>;
  };

  const PERFORMANCE_PROFILES = [
    { value: "default", label: "Default", description: "No tuning, uses WebKit defaults. Recommended for most users." },
    { value: "lite", label: "Lite", description: "Reduced resource usage for low-RAM systems (< 4GB) or battery saving." },
    { value: "max", label: "Max", description: "Maximum responsiveness for heavy pages (GitHub PRs, complex SPAs)." },
    { value: "custom", label: "Custom", description: "Manual control via config file. Edit config.toml to set individual values." },
  ];

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

  // UI state
  let loading = $state(true);
  let loadError = $state<string | null>(null);
  let config = $state<ConfigDTO | null>(null);
  let saving = $state(false);
  let saveSuccess = $state(false);
  let saveError = $state<string | null>(null);
  let showRestartWarning = $state(false);
  let themeEvents = $state(0);
  let activeTab = $state("appearance");

  // Load config from API
  async function loadConfig() {
    loading = true;
    loadError = null;
    try {
      const response = await fetch("/api/config");
      if (!response.ok) throw new Error("Failed to fetch config");
      config = (await response.json()) as ConfigDTO;
    } catch (e: any) {
      loadError = e.message;
      console.error("[config] load failed", e);
    } finally {
      loading = false;
    }
  }

  // Reload config (used after save to get normalized values)
  function reloadConfig() {
    loadConfig();
  }

  let resetDialogOpen = $state(false);

  async function doResetToDefaults() {
    saveSuccess = false;
    try {
      const response = await fetch("/api/config/default");
      if (!response.ok) throw new Error("Failed to fetch default config");
      config = (await response.json()) as ConfigDTO;
    } catch (e: any) {
      console.error("[config] reset defaults failed", e);
    }
    resetDialogOpen = false;
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
    saveError = null;

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
          saveError = "Save timed out (no response from native handler)";
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

        // Show restart warning if performance tab was active
        if (activeTab === "performance") {
          showRestartWarning = true;
        }

        // Refresh config from backend (in case watcher normalized values)
        reloadConfig();

        setTimeout(() => {
          saveSuccess = false;
        }, 3000);
      };
      (window as any).__dumber_config_error = (msg: unknown) => {
        clearSaveTimeout();
        console.error("[config] save error", msg);
        saveError = typeof msg === "string" ? msg : "Failed to save config";
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
      saveError = e.message;
      saving = false;
    }
  }

  function updateShortcuts(nextShortcuts: Record<string, SearchShortcut>) {
    if (!config) return;
    config.search_shortcuts = nextShortcuts;
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
    window.addEventListener("dumber:theme-changed", onThemeChanged);

    // Load config on mount
    loadConfig();

    return () => {
      window.removeEventListener("dumber:theme-changed", onThemeChanged);
    };
  });
</script>

<ConfigShell>
  <div class="flex w-full flex-col gap-6 px-6 py-8">
    {#if loading}
      <!-- Loading state with spinner -->
      <div class="flex flex-col items-center justify-center gap-4 py-16">
        <Spinner class="size-8" />
        <div class="text-xs font-semibold uppercase tracking-[0.4em] text-muted-foreground">
          Loading configuration…
        </div>
      </div>
    {:else if loadError}
      <!-- Error state -->
      <div class="flex flex-col items-center justify-center gap-4 py-16">
        <Card.Root class="max-w-md">
          <Card.Header>
            <Card.Title class="text-destructive">Failed to load config</Card.Title>
            <Card.Description>{loadError}</Card.Description>
          </Card.Header>
          <Card.Content>
            <Button variant="outline" onclick={reloadConfig}>Try again</Button>
          </Card.Content>
        </Card.Root>
      </div>
    {:else if config}
      <Tabs.Root bind:value={activeTab} class="w-full">
        <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div class="text-xs font-semibold uppercase tracking-[0.3em] text-muted-foreground">
            Config Workspace
          </div>
          <Tabs.List class="bg-muted/60">
            <Tabs.Trigger value="appearance">Appearance</Tabs.Trigger>
            <Tabs.Trigger value="search">Search</Tabs.Trigger>
            <Tabs.Trigger value="performance">Performance</Tabs.Trigger>
          </Tabs.List>
        </div>

        <Tabs.Content value="appearance">
          <Card.Root class="rounded-none border-0 bg-transparent py-0 shadow-none">
            <Card.Header>
              <Card.Title>Appearance</Card.Title>
              <Card.Description>Fonts, palette, and theme preferences.</Card.Description>
            </Card.Header>
            <Card.Content class="space-y-8">
              <div class="grid gap-6 md:grid-cols-2">
                <div class="space-y-2">
                  <Label for="sans_font">Sans-Serif Font</Label>
                  <Input id="sans_font" bind:value={config.appearance.sans_font} />
                </div>
                <div class="space-y-2">
                  <Label for="serif_font">Serif Font</Label>
                  <Input id="serif_font" bind:value={config.appearance.serif_font} />
                </div>
                <div class="space-y-2">
                  <Label for="monospace_font">Monospace Font</Label>
                  <Input id="monospace_font" bind:value={config.appearance.monospace_font} />
                </div>
                <div class="space-y-2">
                  <Label for="font_size">Default Font Size</Label>
                  <Input id="font_size" type="number" bind:value={config.appearance.default_font_size} />
                </div>
                <div class="space-y-2">
                  <Label for="color_scheme">Color Scheme</Label>
                  <select
                    id="color_scheme"
                    bind:value={config.appearance.color_scheme}
                    class="flex h-10 w-full border border-border bg-background px-3 py-2 text-sm text-foreground ring-offset-background transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  >
                    <option value="default">Follow System</option>
                    <option value="prefer-dark">Always Dark</option>
                    <option value="prefer-light">Always Light</option>
                  </select>
                </div>
                <div class="space-y-2">
                  <Label for="ui_scale">UI Scale</Label>
                  <Input id="ui_scale" type="number" step="0.1" bind:value={config.default_ui_scale} />
                </div>
              </div>

              <div class="space-y-6">
                <div class="space-y-3">
                  <div class="text-sm font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                    Theme: Light
                  </div>
                  <div class="grid gap-4 md:grid-cols-2">
                    {#each PALETTE_FIELDS as field (field.key)}
                      <div class="space-y-2">
                        <Label for={`light_${field.key}`}>{field.label}</Label>
                        <div class="flex items-center gap-3">
                          <ColorPicker
                            id={`light_${field.key}`}
                            value={config.appearance.light_palette[field.key]}
                            onValueChange={(v) => {
                              config!.appearance.light_palette[field.key] = v;
                            }}
                          />
                          <Input bind:value={config.appearance.light_palette[field.key]} />
                        </div>
                        {#if !isHexColor(config.appearance.light_palette[field.key])}
                          <p class="text-xs text-muted-foreground">Expected hex color like #RRGGBB</p>
                        {/if}
                      </div>
                    {/each}
                  </div>
                </div>

                <div class="space-y-3">
                  <div class="text-sm font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                    Theme: Dark
                  </div>
                  <div class="grid gap-4 md:grid-cols-2">
                    {#each PALETTE_FIELDS as field (field.key)}
                      <div class="space-y-2">
                        <Label for={`dark_${field.key}`}>{field.label}</Label>
                        <div class="flex items-center gap-3">
                          <ColorPicker
                            id={`dark_${field.key}`}
                            value={config.appearance.dark_palette[field.key]}
                            onValueChange={(v) => {
                              config!.appearance.dark_palette[field.key] = v;
                            }}
                          />
                          <Input bind:value={config.appearance.dark_palette[field.key]} />
                        </div>
                        {#if !isHexColor(config.appearance.dark_palette[field.key])}
                          <p class="text-xs text-muted-foreground">Expected hex color like #RRGGBB</p>
                        {/if}
                      </div>
                    {/each}
                  </div>
                </div>
              </div>
            </Card.Content>
          </Card.Root>
        </Tabs.Content>

        <Tabs.Content value="search">
          <Card.Root class="rounded-none border-0 bg-transparent py-0 shadow-none">
            <Card.Header>
              <Card.Title>Search</Card.Title>
              <Card.Description>Default engine and shortcuts.</Card.Description>
            </Card.Header>
            <Card.Content class="space-y-8">
              <div class="space-y-2">
                <Label for="search_engine">Default Search Engine (use %s for query)</Label>
                <Input id="search_engine" bind:value={config.default_search_engine} />
              </div>

              <div class="space-y-3">
                <div class="text-sm font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                  Search Shortcuts
                </div>
                <p class="text-sm text-muted-foreground">Map prefixes like g to URL templates.</p>
                <ShortcutsTable shortcuts={config.search_shortcuts} onUpdate={updateShortcuts} />
              </div>
            </Card.Content>
          </Card.Root>
        </Tabs.Content>

        <Tabs.Content value="performance">
          <Card.Root class="rounded-none border-0 bg-transparent py-0 shadow-none">
            <Card.Header>
              <Card.Title>Performance Profile</Card.Title>
              <Card.Description>
                WebKitGTK rendering and memory tuning presets.
              </Card.Description>
            </Card.Header>
            <Card.Content class="space-y-6">
              <!-- Experimental feature warning -->
              <div class="flex items-start gap-3 rounded-md border border-info/30 bg-info/10 px-4 py-3">
                <span class="text-info">🧪</span>
                <div class="space-y-1">
                  <p class="text-sm font-medium text-info">Experimental Feature</p>
                  <p class="text-xs text-muted-foreground">
                    Performance profiles are experimental. Results may vary depending on your hardware and WebKitGTK version.
                  </p>
                </div>
              </div>

              <!-- Profile selector -->
              {#if config.performance}
              <div class="space-y-4">
                {#each PERFORMANCE_PROFILES as profile (profile.value)}
                  <label
                    class="flex cursor-pointer items-start gap-4 rounded-md border p-4 transition-colors hover:bg-muted/50 {config.performance.profile === profile.value ? 'border-primary bg-primary/5' : ''}"
                  >
                    <input
                      type="radio"
                      name="performance_profile"
                      value={profile.value}
                      checked={config.performance.profile === profile.value}
                      onchange={() => config!.performance.profile = profile.value}
                      class="mt-1 accent-primary"
                    />
                    <div class="space-y-1">
                      <div class="font-medium">{profile.label}</div>
                      <div class="text-sm text-muted-foreground">{profile.description}</div>
                    </div>
                  </label>
                {/each}
              </div>
              {:else}
              <div class="text-sm text-muted-foreground">
                Performance settings not available. This may be due to a configuration error.
              </div>
              {/if}

              <!-- Custom profile note -->
              {#if config.performance?.profile === "custom"}
                <div class="rounded-md border border-border bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
                  <p>
                    Custom profile settings are configured in your <code class="rounded bg-muted px-1 py-0.5">config.toml</code> file.
                    See the <a href="https://github.com/bnema/dumber/blob/main/docs/CONFIG.md#performance-profiles" target="_blank" class="text-primary underline">documentation</a> for available options.
                  </p>
                </div>
              {/if}

              <!-- Resolved values display -->
              {#if config.performance?.resolved}
                <div class="space-y-3">
                  <div class="text-sm font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                    Resolved Settings (applied on restart)
                  </div>
                  <div class="grid gap-3 rounded-md border border-border bg-muted/20 p-4 text-sm md:grid-cols-2">
                    <div class="flex justify-between">
                      <span class="text-muted-foreground">Skia CPU Threads</span>
                      <span class="font-mono">{config.performance.resolved.skia_cpu_threads === 0 ? 'unset' : config.performance.resolved.skia_cpu_threads}</span>
                    </div>
                    <div class="flex justify-between">
                      <span class="text-muted-foreground">Skia GPU Threads</span>
                      <span class="font-mono">{config.performance.resolved.skia_gpu_threads === -1 ? 'unset' : config.performance.resolved.skia_gpu_threads}</span>
                    </div>
                    <div class="flex justify-between">
                      <span class="text-muted-foreground">Web Process Memory</span>
                      <span class="font-mono">{config.performance.resolved.web_process_memory_mb === 0 ? 'unset' : config.performance.resolved.web_process_memory_mb + ' MB'}</span>
                    </div>
                    <div class="flex justify-between">
                      <span class="text-muted-foreground">Network Process Memory</span>
                      <span class="font-mono">{config.performance.resolved.network_process_memory_mb === 0 ? 'unset' : config.performance.resolved.network_process_memory_mb + ' MB'}</span>
                    </div>
                    <div class="flex justify-between">
                      <span class="text-muted-foreground">WebView Pool Prewarm</span>
                      <span class="font-mono">{config.performance.resolved.webview_pool_prewarm}</span>
                    </div>
                    <div class="flex justify-between">
                      <span class="text-muted-foreground">Memory Kill Threshold</span>
                      <span class="font-mono">{config.performance.resolved.kill_threshold === -1 ? 'never' : (config.performance.resolved.kill_threshold * 100).toFixed(0) + '%'}</span>
                    </div>
                  </div>
                </div>
              {/if}
            </Card.Content>
          </Card.Root>
        </Tabs.Content>
      </Tabs.Root>

      <div class="flex flex-wrap items-center justify-between gap-4 border-t border-border px-6 py-4">
        <div class="flex items-center gap-3 text-xs uppercase tracking-[0.3em] text-muted-foreground">
          {#if saveSuccess}
            <span class="text-primary">Saved</span>
          {:else}
            <span>Ready</span>
          {/if}
          {#if themeEvents > 0}
            <span>Theme x{themeEvents}</span>
          {/if}
        </div>
        <div class="flex items-center gap-3">
          <AlertDialog.Root bind:open={resetDialogOpen}>
            <AlertDialog.Trigger disabled={saving}>
              {#snippet child({ props })}
                <Button variant="outline" {...props} type="button">
                  Reset Defaults
                </Button>
              {/snippet}
            </AlertDialog.Trigger>
            <AlertDialog.Content>
              <AlertDialog.Header>
                <AlertDialog.Title>Reset to defaults?</AlertDialog.Title>
                <AlertDialog.Description>
                  This will reset all settings on this page to their default values.
                  You will still need to click Save to persist the changes.
                </AlertDialog.Description>
              </AlertDialog.Header>
              <AlertDialog.Footer>
                <AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
                <AlertDialog.Action onclick={doResetToDefaults}>
                  Reset Defaults
                </AlertDialog.Action>
              </AlertDialog.Footer>
            </AlertDialog.Content>
          </AlertDialog.Root>
          <Button onclick={saveConfig} disabled={saving} type="button">
            {saving ? "Saving…" : "Save"}
          </Button>
        </div>
      </div>

      <!-- Save error display -->
      {#if saveError}
        <div class="border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {saveError}
        </div>
      {/if}

      <!-- Restart warning after saving performance settings -->
      {#if showRestartWarning}
        <div class="flex items-start gap-3 border border-warning/30 bg-warning/10 px-4 py-3">
          <span class="text-warning">⚠</span>
          <div class="space-y-1">
            <p class="text-sm font-medium text-warning">Restart Required</p>
            <p class="text-xs text-muted-foreground">
              Performance settings are applied at browser startup. Restart Dumber for changes to take effect.
            </p>
          </div>
        </div>
      {/if}
    {/if}
  </div>
</ConfigShell>
