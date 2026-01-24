<script lang="ts" module>
	export type ColorPickerProps = {
		value: string;
		onValueChange?: (value: string) => void;
		id?: string;
		class?: string;
	};
</script>

<script lang="ts">
	import ColorPicker from "svelte-awesome-color-picker";
	import { Popover } from "bits-ui";
	import { cn } from "$lib/utils.js";

	let {
		class: className,
		value = $bindable("#000000"),
		onValueChange,
		id,
	}: ColorPickerProps = $props();

	let open = $state(false);

	// svelte-awesome-color-picker uses hex binding - use writable derived to sync with value
	let hex = $derived.by(() => value);

	function handleInput(event: { hex: string | null }) {
		if (event.hex) {
			value = event.hex;
			onValueChange?.(event.hex);
		}
	}
</script>

<Popover.Root bind:open>
	<Popover.Trigger
		{id}
		class={cn(
			"inline-flex h-10 w-12 cursor-pointer items-center justify-center border border-border bg-background transition-colors hover:border-foreground/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
			className
		)}
	>
		<span
			class="block h-6 w-8 border border-border/50"
			style="background-color: {value};"
			aria-hidden="true"
		></span>
		<span class="sr-only">Choose color: {value}</span>
	</Popover.Trigger>

	<Popover.Portal>
		<Popover.Content
			class="color-picker-popover z-50 border border-border bg-background p-3 shadow-lg"
			sideOffset={4}
		>
			<ColorPicker
				bind:hex
				onInput={handleInput}
				isDialog={false}
			/>
			<!-- Hex value display -->
			<div class="mt-2 flex items-center gap-2">
				<span
					class="block h-6 w-6 shrink-0 border border-border"
					style="background-color: {hex};"
					aria-hidden="true"
				></span>
				<span class="font-mono text-sm text-foreground">{hex}</span>
			</div>
		</Popover.Content>
	</Popover.Portal>
</Popover.Root>

<style>
	/* Override svelte-awesome-color-picker styles to match our theme */
	:global(.color-picker-popover) {
		--cp-bg-color: var(--background);
		--cp-border-color: var(--border);
		--cp-text-color: var(--foreground);
		--cp-input-color: var(--background);
		--cp-button-hover-color: var(--accent);
	}
</style>
