<script lang="ts">
  import { onMount } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { Spinner } from "$lib/components/ui/spinner";
  import * as Card from "$lib/components/ui/card";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import { RefreshCw, RotateCcw, AlertTriangle } from "@lucide/svelte";
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

  let loading = $state(true);
  let error = $state<string | null>(null);
  let keybindings = $state<KeybindingsConfig | null>(null);
  let editingBinding = $state<{ mode: string; action: string; entry: KeybindingEntry } | null>(null);
  let resetAllDialogOpen = $state(false);

  function getWebKitBridge(): { postMessage: (msg: unknown) => void } | null {
    const bridge = (window as any).webkit?.messageHandlers?.dumber;
    return bridge && typeof bridge.postMessage === "function" ? bridge : null;
  }

  function getWebViewId(): number {
    return (window as any).__dumber_webview_id || 0;
  }

  async function loadKeybindings() {
    loading = true;
    error = null;

    const bridge = getWebKitBridge();
    if (!bridge) {
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

    bridge.postMessage({
      type: "get_keybindings",
      webviewId: getWebViewId(),
      payload: {},
    });
  }

  function startEdit(mode: string, action: string, entry: KeybindingEntry) {
    editingBinding = { mode, action, entry };
  }

  async function saveBinding(keys: string[]) {
    console.log("[DEBUG] saveBinding called with keys:", keys, "editingBinding:", editingBinding);
    if (!editingBinding) {
      console.log("[DEBUG] saveBinding: no editingBinding, returning");
      return;
    }

    const bridge = getWebKitBridge();
    console.log("[DEBUG] saveBinding: bridge =", bridge);
    if (!bridge) {
      console.log("[DEBUG] saveBinding: no bridge, returning");
      return;
    }

    (window as any).__dumber_keybinding_set = () => {
      loadKeybindings();
      editingBinding = null;
    };
    (window as any).__dumber_keybinding_error = (msg: string) => {
      error = typeof msg === "string" ? msg : "Failed to save keybinding";
    };

    bridge.postMessage({
      type: "set_keybinding",
      webviewId: getWebViewId(),
      payload: {
        mode: editingBinding.mode,
        action: editingBinding.action,
        keys,
      },
    });
  }

  async function resetBinding(mode: string, action: string) {
    const bridge = getWebKitBridge();
    if (!bridge) return;

    (window as any).__dumber_keybinding_reset = () => {
      loadKeybindings();
    };
    (window as any).__dumber_keybinding_error = (msg: string) => {
      error = typeof msg === "string" ? msg : "Failed to reset keybinding";
    };

    bridge.postMessage({
      type: "reset_keybinding",
      webviewId: getWebViewId(),
      payload: { mode, action },
    });
  }

  async function resetAllBindings() {
    const bridge = getWebKitBridge();
    if (!bridge) return;

    (window as any).__dumber_keybindings_reset_all = () => {
      loadKeybindings();
      resetAllDialogOpen = false;
    };
    (window as any).__dumber_keybinding_error = (msg: string) => {
      error = typeof msg === "string" ? msg : "Failed to reset keybindings";
    };

    bridge.postMessage({
      type: "reset_all_keybindings",
      webviewId: getWebViewId(),
      payload: {},
    });
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
    {:else if keybindings}
      {#each keybindings.groups as group (group.mode)}
        <KeybindingGroup
          {group}
          onEdit={(action, entry) => startEdit(group.mode, action, entry)}
          onReset={(action) => resetBinding(group.mode, action)}
        />
      {/each}
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
