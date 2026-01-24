<script lang="ts">
  import { onMount } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { Spinner } from "$lib/components/ui/spinner";
  import * as Card from "$lib/components/ui/card";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import { RefreshCw, RotateCcw, AlertTriangle, CheckCircle } from "@lucide/svelte";
  import { postMessage, getWebKitBridge, getWebViewId } from "$lib/bridge";
  import KeybindingGroup from "./KeybindingGroup.svelte";
  import KeyCaptureModal from "./KeyCaptureModal.svelte";

  type KeybindingEntry = {
    action: string;
    description: string;
    keys: string[];
    default_keys: string[];
    is_custom: boolean;
  };

  type KeybindingGroupData = {
    mode: string;
    display_name: string;
    bindings: KeybindingEntry[];
    activation?: string;
  };

  type KeybindingsConfig = {
    groups: KeybindingGroupData[];
  };

  type KeybindingConflict = {
    conflicting_action: string;
    conflicting_mode: string;
    key: string;
  };

  let loading = $state(true);
  let error = $state<string | null>(null);
  let keybindings = $state<KeybindingsConfig | null>(null);
  let editingBinding = $state<{ mode: string; action: string; entry: KeybindingEntry } | null>(null);
  let resetAllDialogOpen = $state(false);
  let successMessage = $state<string | null>(null);
  let conflicts = $state<KeybindingConflict[]>([]);

  async function loadKeybindings() {
    loading = true;
    error = null;

    if (!getWebKitBridge()) {
      error = "WebKit bridge not available";
      loading = false;
      return;
    }

    (window as any).__dumber_keybindings_loaded = (data: KeybindingsConfig) => {
      keybindings = data;
      loading = false;
    };
    (window as any).__dumber_keybindings_error = (msg: string) => {
      error = typeof msg === "string" ? msg : "Failed to load keybindings";
      loading = false;
    };

    try {
      postMessage({
        type: "get_keybindings",
        webviewId: getWebViewId(),
        payload: {},
      });
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to load keybindings";
      loading = false;
    }
  }

  function startEdit(mode: string, action: string, entry: KeybindingEntry) {
    editingBinding = { mode, action, entry };
  }

  function showSuccess(message: string) {
    successMessage = message;
    setTimeout(() => { successMessage = null; }, 3000);
  }

  async function saveBinding(keys: string[]) {
    if (!editingBinding) {
      return;
    }

    if (!getWebKitBridge()) {
      error = "WebKit bridge not available";
      return;
    }

    (window as any).__dumber_keybinding_set = (response: { status: string; conflicts?: KeybindingConflict[] }) => {
      console.log("[KeybindingsTab] Received set_keybinding response:", response);
      // Verify we got a valid success response from backend
      if (!response || response.status !== "success") {
        error = "Failed to save keybinding: invalid response";
        console.error("[KeybindingsTab] Invalid response:", response);
        return;
      }
      if (response.conflicts && response.conflicts.length > 0) {
        conflicts = response.conflicts;
      } else {
        conflicts = [];
      }
      loadKeybindings();
      editingBinding = null;
      showSuccess("Keybinding saved successfully");
    };
    (window as any).__dumber_keybinding_error = (msg: string) => {
      console.error("[KeybindingsTab] set_keybinding error:", msg);
      error = typeof msg === "string" ? msg : "Failed to save keybinding";
    };

    try {
      console.log("[KeybindingsTab] Sending set_keybinding:", {
        mode: editingBinding.mode,
        action: editingBinding.action,
        keys: keys,
      });
      // postMessage handles Svelte 5 Proxy serialization automatically
      postMessage({
        type: "set_keybinding",
        webviewId: getWebViewId(),
        payload: {
          mode: editingBinding.mode,
          action: editingBinding.action,
          keys: keys,
        },
      });
      console.log("[KeybindingsTab] Message posted, waiting for callback...");
    } catch (err) {
      console.error("[KeybindingsTab] postMessage failed:", err);
      error = err instanceof Error ? err.message : "Failed to save keybinding";
    }
  }

  async function resetBinding(mode: string, action: string) {
    if (!getWebKitBridge()) return;

    (window as any).__dumber_keybinding_reset = () => {
      loadKeybindings();
    };
    (window as any).__dumber_keybinding_error = (msg: string) => {
      error = typeof msg === "string" ? msg : "Failed to reset keybinding";
    };

    try {
      postMessage({
        type: "reset_keybinding",
        webviewId: getWebViewId(),
        payload: { mode, action },
      });
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to reset keybinding";
    }
  }

  async function resetAllBindings() {
    if (!getWebKitBridge()) return;

    (window as any).__dumber_keybindings_reset_all = () => {
      loadKeybindings();
      resetAllDialogOpen = false;
    };
    (window as any).__dumber_keybinding_error = (msg: string) => {
      error = typeof msg === "string" ? msg : "Failed to reset keybindings";
    };

    try {
      postMessage({
        type: "reset_all_keybindings",
        webviewId: getWebViewId(),
        payload: {},
      });
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to reset keybindings";
    }
  }

  onMount(() => {
    loadKeybindings();
  });
</script>

<Card.Root class="rounded-none border-0 bg-transparent py-0 shadow-none">
  <Card.Header>
    <div class="flex items-center justify-between">
      <div>
        <Card.Title>Keybindings</Card.Title>
        <Card.Description>
          Customize keyboard shortcuts for all modes.
        </Card.Description>
      </div>
      <div class="flex gap-2">
        <Button variant="outline" size="sm" onclick={loadKeybindings}>
          <RefreshCw class="mr-1 size-4" />
          Refresh
        </Button>
        <AlertDialog.Root bind:open={resetAllDialogOpen}>
          <AlertDialog.Trigger>
            {#snippet child({ props })}
              <Button variant="outline" size="sm" {...props}>
                <RotateCcw class="mr-1 size-4" />
                Reset All
              </Button>
            {/snippet}
          </AlertDialog.Trigger>
          <AlertDialog.Content>
            <AlertDialog.Header>
              <AlertDialog.Title>Reset all keybindings?</AlertDialog.Title>
              <AlertDialog.Description>
                This will restore all keybindings to their default values. This action cannot be undone.
              </AlertDialog.Description>
            </AlertDialog.Header>
            <AlertDialog.Footer>
              <AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
              <AlertDialog.Action onclick={resetAllBindings}>Reset All</AlertDialog.Action>
            </AlertDialog.Footer>
          </AlertDialog.Content>
        </AlertDialog.Root>
      </div>
    </div>
  </Card.Header>
  <Card.Content class="space-y-6">
    {#if loading}
      <div class="flex items-center justify-center py-8">
        <Spinner class="size-6" />
      </div>
    {:else if error}
      <div class="flex items-center gap-2 rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
        <AlertTriangle class="size-4" />
        {error}
      </div>
    {/if}
    {#if successMessage}
      <div class="flex items-center gap-2 rounded-md border border-green-500/50 bg-green-500/10 px-4 py-3 text-sm text-green-600 dark:text-green-400">
        <CheckCircle class="size-4" />
        {successMessage}
      </div>
    {/if}
    {#if conflicts.length > 0}
      <div class="rounded-md border border-yellow-500/50 bg-yellow-500/10 px-4 py-3 text-sm">
        <div class="flex items-center gap-2 text-yellow-600 dark:text-yellow-400">
          <AlertTriangle class="size-4" />
          <span class="font-medium">Keybinding conflicts detected</span>
        </div>
        <ul class="mt-2 list-disc pl-6 text-muted-foreground">
          {#each conflicts as conflict}
            <li>
              <kbd class="font-mono text-xs">{conflict.key}</kbd> is also bound to
              <strong>{conflict.conflicting_action}</strong> in <em>{conflict.conflicting_mode}</em> mode
            </li>
          {/each}
        </ul>
        <button
          type="button"
          class="mt-2 text-xs text-muted-foreground hover:text-foreground"
          onclick={() => conflicts = []}
        >
          Dismiss
        </button>
      </div>
    {/if}
    {#if !loading && !error && keybindings}
      {#each keybindings.groups as group (group.mode)}
        <KeybindingGroup
          {group}
          onEdit={(action, entry) => startEdit(group.mode, action, entry)}
          onReset={(action) => resetBinding(group.mode, action)}
        />
      {/each}
    {/if}
    {#if !loading && !keybindings && !error}
      <div class="text-center text-muted-foreground py-8">No keybindings available</div>
    {/if}
  </Card.Content>
</Card.Root>

{#if editingBinding}
  <KeyCaptureModal
    binding={editingBinding.entry}
    onSave={saveBinding}
    onCancel={() => (editingBinding = null)}
  />
{/if}
