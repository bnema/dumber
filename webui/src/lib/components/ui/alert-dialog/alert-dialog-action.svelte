<script lang="ts">
	import { AlertDialog as AlertDialogPrimitive } from "bits-ui";
	import { buttonVariants } from "$lib/components/ui/button/index.js";
	import { cn } from "$lib/utils.js";
	import type { Snippet } from "svelte";
	import type { MouseEventHandler } from "svelte/elements";

	let {
		ref = $bindable(null),
		class: className,
		children,
		onclick,
		...restProps
	}: AlertDialogPrimitive.ActionProps & {
		children?: Snippet;
		onclick?: MouseEventHandler<HTMLButtonElement>;
	} = $props();
</script>

<AlertDialogPrimitive.Action
	bind:ref
	class={cn(buttonVariants(), className)}
	{onclick}
	{...restProps}
>
	{@render children?.()}
</AlertDialogPrimitive.Action>
