<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import * as Table from "$lib/components/ui/table";
  import { RotateCcw, Edit3 } from "@lucide/svelte";
  import { formatKey } from "$lib/utils/keys";

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

  interface Props {
    group: KeybindingGroupData;
    onEdit: (action: string, entry: KeybindingEntry) => void;
    onReset: (action: string) => void;
  }

  let { group, onEdit, onReset }: Props = $props();
</script>

<div class="space-y-3">
  <div class="flex items-center gap-2">
    <h3 class="text-sm font-semibold uppercase tracking-[0.15em] text-muted-foreground">
      {group.display_name}
    </h3>
    {#if group.activation}
      <span class="rounded-md bg-muted px-2 py-0.5 text-xs font-mono text-muted-foreground">
        {formatKey(group.activation)}
      </span>
    {/if}
  </div>

  <Table.Root class="rounded-md border border-border">
    <Table.Header>
      <Table.Row>
        <Table.Head class="w-[180px]">Action</Table.Head>
        <Table.Head>Description</Table.Head>
        <Table.Head class="w-[200px]">Keybinding</Table.Head>
        <Table.Head class="w-[100px] text-right">Actions</Table.Head>
      </Table.Row>
    </Table.Header>
    <Table.Body>
      {#each group.bindings as binding (binding.action)}
        <Table.Row class={binding.is_custom ? "bg-accent/5" : ""}>
          <Table.Cell class="font-mono text-sm">{binding.action}</Table.Cell>
          <Table.Cell class="text-muted-foreground">{binding.description}</Table.Cell>
          <Table.Cell>
            <div class="flex flex-wrap gap-1">
              {#each binding.keys as key (key)}
                <kbd class="rounded bg-muted px-1.5 py-0.5 text-xs font-mono">
                  {formatKey(key)}
                </kbd>
              {/each}
              {#if binding.keys.length === 0}
                <span class="text-xs text-muted-foreground">(unbound)</span>
              {/if}
            </div>
          </Table.Cell>
          <Table.Cell class="text-right">
            <div class="flex justify-end gap-1">
              <Button
                variant="ghost"
                size="icon"
                onclick={() => onEdit(binding.action, binding)}
                aria-label="Edit binding"
              >
                <Edit3 class="size-4" />
              </Button>
              {#if binding.is_custom}
                <Button
                  variant="ghost"
                  size="icon"
                  onclick={() => onReset(binding.action)}
                  aria-label="Reset to default"
                >
                  <RotateCcw class="size-4" />
                </Button>
              {/if}
            </div>
          </Table.Cell>
        </Table.Row>
      {/each}
    </Table.Body>
  </Table.Root>
</div>
