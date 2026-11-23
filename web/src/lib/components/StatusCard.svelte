<!--
@component
A state the page watches, on a raised card: while `waiting` a PulseDot, once done a check in a
soft green circle; its title, a line under it and actions at the right end. A live region, so a
change is read out.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';
	import PulseDot from './PulseDot.svelte';

	type Props = {
		state: 'waiting' | 'done';
		title: string;
		children: Snippet;
		actions?: Snippet;
	};

	let { state, title, children, actions }: Props = $props();
</script>

<section
	class="mt-8 flex max-w-guide flex-wrap items-center gap-x-5 gap-y-4 rounded-dialog border border-border bg-surface px-6 py-[22px] shadow-pop"
	role="status"
	aria-live="polite"
>
	{#if state === 'waiting'}
		<PulseDot />
	{:else}
		<span
			class="flex size-9 shrink-0 items-center justify-center rounded-full bg-success-bg text-success"
			aria-hidden="true"
		>
			<Icon name="check" size={18} />
		</span>
	{/if}
	<div class="min-w-60 flex-1">
		<p class="text-lg font-bold">{title}</p>
		<p class="text-muted">{@render children()}</p>
	</div>
	{#if actions}
		<div class="flex flex-wrap items-center gap-2">{@render actions()}</div>
	{/if}
</section>
