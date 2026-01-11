<script lang="ts">
  import { onMount } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import { Keyboard, X, Plus } from "@lucide/svelte";

  type KeybindingEntry = {
    action: string;
    description: string;
    keys: string[];
    default_keys: string[];
    is_custom: boolean;
  };

  interface Props {
    binding: KeybindingEntry;
    onSave: (keys: string[]) => void;
    onCancel: () => void;
  }

  let { binding, onSave, onCancel }: Props = $props();

  // Use $derived for initial keys to maintain reactivity if binding changes
  let newKeys = $state<string[]>([]);
  let isCapturing = $state(false);
  let capturedKey = $state<string | null>(null);
  let initialized = $state(false);

  // Initialize keys from binding on mount
  $effect(() => {
    if (!initialized) {
      newKeys = [...binding.keys];
      initialized = true;
    }
  });

  function handleKeyDown(e: KeyboardEvent) {
    if (!isCapturing) return;
    e.preventDefault();
    e.stopPropagation();

    const parts: string[] = [];
    if (e.ctrlKey) parts.push("ctrl");
    if (e.altKey) parts.push("alt");
    if (e.shiftKey) parts.push("shift");

    let key = e.key.toLowerCase();
    switch (key) {
      case "control":
      case "alt":
      case "shift":
      case "meta":
        return; // Modifier only, wait for actual key
      case "arrowleft":
        key = "arrowleft";
        break;
      case "arrowright":
        key = "arrowright";
        break;
      case "arrowup":
        key = "arrowup";
        break;
      case "arrowdown":
        key = "arrowdown";
        break;
      case "escape":
        isCapturing = false;
        capturedKey = null;
        return;
      case " ":
        key = "space";
        break;
      case "enter":
        key = "enter";
        break;
      case "tab":
        key = "tab";
        break;
      case "backspace":
        key = "backspace";
        break;
    }

    parts.push(key);
    capturedKey = parts.join("+");
    isCapturing = false;
  }

  function addCapturedKey() {
    if (capturedKey && !newKeys.includes(capturedKey)) {
      newKeys = [...newKeys, capturedKey];
    }
    capturedKey = null;
  }

  function removeKey(index: number) {
    newKeys = newKeys.filter((_, i) => i !== index);
  }

  function formatKey(key: string): string {
    return key
      .replace(/arrowleft/gi, "Left")
      .replace(/arrowright/gi, "Right")
      .replace(/arrowup/gi, "Up")
      .replace(/arrowdown/gi, "Down")
      .replace(/\+/g, " + ")
      .replace(/ctrl/gi, "Ctrl")
      .replace(/alt/gi, "Alt")
      .replace(/shift/gi, "Shift");
  }

  onMount(() => {
    const handler = (e: KeyboardEvent) => handleKeyDown(e);
    window.addEventListener("keydown", handler, true);
    return () => window.removeEventListener("keydown", handler, true);
  });
</script>

<AlertDialog.Root open={true} onOpenChange={(open) => !open && onCancel()}>
  <AlertDialog.Content class="max-w-md">
    <AlertDialog.Header>
      <AlertDialog.Title>Edit Keybinding</AlertDialog.Title>
      <AlertDialog.Description>
        Configure keybindings for <strong>{binding.action}</strong>
      </AlertDialog.Description>
    </AlertDialog.Header>

    <div class="space-y-4 py-4">
      <!-- Current bindings -->
      <div class="space-y-2">
        <div class="text-xs font-semibold uppercase tracking-[0.15em] text-muted-foreground">
          Current Bindings
        </div>
        <div class="flex flex-wrap gap-2">
          {#each newKeys as key, i}
            <div class="flex items-center gap-1 rounded bg-muted px-2 py-1">
              <kbd class="text-xs font-mono">{formatKey(key)}</kbd>
              <button
                type="button"
                onclick={() => removeKey(i)}
                class="text-muted-foreground hover:text-foreground"
              >
                <X class="size-3" />
              </button>
            </div>
          {/each}
          {#if newKeys.length === 0}
            <span class="text-sm text-muted-foreground">(no bindings)</span>
          {/if}
        </div>
      </div>

      <!-- Capture area -->
      <div class="space-y-2">
        <div class="text-xs font-semibold uppercase tracking-[0.15em] text-muted-foreground">
          Add New Binding
        </div>
        <div class="flex gap-2">
          <button
            type="button"
            onclick={() => { isCapturing = true; capturedKey = null; }}
            class="flex flex-1 items-center justify-center gap-2 rounded-md border border-dashed border-border px-4 py-6 text-sm text-muted-foreground transition hover:border-foreground hover:text-foreground {isCapturing ? 'border-primary bg-primary/5 text-primary' : ''}"
          >
            <Keyboard class="size-5" />
            {#if isCapturing}
              Press a key combination...
            {:else if capturedKey}
              <kbd class="font-mono">{formatKey(capturedKey)}</kbd>
            {:else}
              Click to capture key
            {/if}
          </button>
          {#if capturedKey}
            <Button variant="outline" onclick={addCapturedKey}>
              <Plus class="mr-1 size-4" />
              Add
            </Button>
          {/if}
        </div>
      </div>

      <!-- Default hint -->
      {#if binding.default_keys.length > 0}
        <div class="text-xs text-muted-foreground">
          Default: {binding.default_keys.map(formatKey).join(", ")}
        </div>
      {/if}
    </div>

    <AlertDialog.Footer>
      <AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
      <AlertDialog.Action onclick={() => onSave(newKeys)}>Save</AlertDialog.Action>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
