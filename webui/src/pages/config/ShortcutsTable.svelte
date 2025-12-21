<script lang="ts">
  import { Check, Plus, Trash2, X } from "@lucide/svelte";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import * as Table from "$lib/components/ui/table";

  type SearchShortcut = {
    url: string;
    description: string;
  };

  interface Props {
    shortcuts: Record<string, SearchShortcut>;
    onUpdate: (shortcuts: Record<string, SearchShortcut>) => void;
  }

  let { shortcuts, onUpdate }: Props = $props();

  let editingKey = $state<string | null>(null);
  let draftKey = $state("");
  let draftUrl = $state("");
  let draftDescription = $state("");

  let newKey = $state("");
  let newUrl = $state("");
  let newDescription = $state("");

  const entries = $derived(
    Object.entries(shortcuts ?? {}).sort(([a], [b]) => a.localeCompare(b)),
  );

  function startEdit(key: string, shortcut: SearchShortcut) {
    editingKey = key;
    draftKey = key;
    draftUrl = shortcut.url;
    draftDescription = shortcut.description;
  }

  function cancelEdit() {
    editingKey = null;
    draftKey = "";
    draftUrl = "";
    draftDescription = "";
  }

  function commitEdit() {
    if (!editingKey) return;
    const nextKey = draftKey.trim();
    const nextUrl = draftUrl.trim();
    const nextDescription = draftDescription.trim();

    const updated: Record<string, SearchShortcut> = { ...shortcuts };
    delete updated[editingKey];
    if (nextKey) {
      updated[nextKey] = { url: nextUrl, description: nextDescription };
    }
    onUpdate(updated);
    cancelEdit();
  }

  function removeShortcut(key: string) {
    const updated: Record<string, SearchShortcut> = { ...shortcuts };
    delete updated[key];
    onUpdate(updated);
  }

  function addShortcut() {
    const key = newKey.trim();
    if (!key) return;
    const updated: Record<string, SearchShortcut> = { ...shortcuts };
    updated[key] = { url: newUrl.trim(), description: newDescription.trim() };
    onUpdate(updated);
    newKey = "";
    newUrl = "";
    newDescription = "";
  }
</script>

<div class="space-y-4">
  <Table.Root class="rounded-md border border-border">
    <Table.Header>
      <Table.Row>
        <Table.Head class="w-[140px]">Prefix</Table.Head>
        <Table.Head>URL Template</Table.Head>
        <Table.Head>Description</Table.Head>
        <Table.Head class="w-[160px] text-right">Actions</Table.Head>
      </Table.Row>
    </Table.Header>
    <Table.Body>
      {#if entries.length === 0}
        <Table.Row>
          <Table.Cell colspan={4} class="py-8 text-center text-sm text-muted-foreground">
            No shortcuts yet. Add one below.
          </Table.Cell>
        </Table.Row>
      {/if}
      {#each entries as [key, shortcut] (key)}
        <Table.Row>
          {#if editingKey === key}
            <Table.Cell>
              <Input bind:value={draftKey} class="h-8" />
            </Table.Cell>
            <Table.Cell>
              <Input bind:value={draftUrl} class="h-8" />
            </Table.Cell>
            <Table.Cell>
              <Input bind:value={draftDescription} class="h-8" />
            </Table.Cell>
            <Table.Cell class="text-right">
              <div class="flex justify-end gap-2">
                <Button variant="ghost" size="icon" onclick={commitEdit} type="button" aria-label="Save">
                  <Check size={16} />
                </Button>
                <Button variant="ghost" size="icon" onclick={cancelEdit} type="button" aria-label="Cancel">
                  <X size={16} />
                </Button>
              </div>
            </Table.Cell>
          {:else}
            <Table.Cell class="font-mono text-sm">{key}</Table.Cell>
            <Table.Cell class="truncate max-w-[360px]">{shortcut.url}</Table.Cell>
            <Table.Cell class="text-muted-foreground">{shortcut.description}</Table.Cell>
            <Table.Cell class="text-right">
              <div class="flex justify-end gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onclick={() => startEdit(key, shortcut)}
                  type="button"
                >
                  Edit
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  onclick={() => removeShortcut(key)}
                  type="button"
                  aria-label={`Delete ${key}`}
                >
                  <Trash2 size={16} />
                </Button>
              </div>
            </Table.Cell>
          {/if}
        </Table.Row>
      {/each}
    </Table.Body>
  </Table.Root>

  <div class="grid gap-3 md:grid-cols-[140px_1fr_1fr_140px] items-end">
    <div class="space-y-1">
      <span class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">Prefix</span>
      <Input bind:value={newKey} placeholder="g" />
    </div>
    <div class="space-y-1">
      <span class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">URL Template</span>
      <Input bind:value={newUrl} placeholder="https://www.google.com/search?q=%s" />
    </div>
    <div class="space-y-1">
      <span class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">Description</span>
      <Input bind:value={newDescription} placeholder="Google" />
    </div>
    <div class="flex md:justify-end">
      <Button variant="default" onclick={addShortcut} type="button" class="w-full md:w-auto">
        <Plus size={16} />
        Add Shortcut
      </Button>
    </div>
  </div>
</div>
