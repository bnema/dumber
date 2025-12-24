<script lang="ts">
	import { AlertDialog as AlertDialogPrimitive } from "bits-ui";
	import { cn } from "$lib/utils.js";

	let {
		ref = $bindable(null),
		class: className,
		...restProps
	}: AlertDialogPrimitive.TriggerProps = $props();
</script>

<AlertDialogPrimitive.Trigger
	bind:ref
	class={cn(className)}
	{...restProps}
/>
