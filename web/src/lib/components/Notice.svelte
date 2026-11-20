<!--
@component
A tinted note with a bar in its tone: a success, a warning or an error.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { NoticeTone } from './notice.ts';
	import Icon from './Icon.svelte';

	type Props = { tone: NoticeTone; children: Snippet };

	let { tone, children }: Props = $props();

	const tones: Record<NoticeTone, { box: string; icon: string }> = {
		success: { box: 'bg-success-bg before:bg-success', icon: 'text-success' },
		warning: { box: 'bg-warning-bg before:bg-warning', icon: 'text-warning' },
		error: { box: 'bg-error-bg before:bg-error', icon: 'text-error' }
	};
</script>

<div
	class={[
		'relative flex items-start gap-2 rounded-popup py-3 pr-4 pl-5 text-text forced-colors:border',
		'before:absolute before:inset-y-2 before:left-0 before:w-1 before:rounded-r-full',
		tones[tone].box
	]}
>
	<span class={['mt-0.5', tones[tone].icon]}><Icon name={tone} /></span>
	<div class="min-w-0">{@render children()}</div>
</div>
