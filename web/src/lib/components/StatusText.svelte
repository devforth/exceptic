<!--
@component
A state as a quiet pill with a dot in its tone: on, done or sent (success), running or retrying
(warning), failed (error), off, paused or waiting (muted).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { StatusTone } from './status.ts';

	type Props = { tone: StatusTone; title?: string; children: Snippet };

	let { tone, title, children }: Props = $props();

	const tones: Record<StatusTone, string> = {
		success: 'bg-success-bg text-success',
		warning: 'bg-warning-bg text-warning',
		error: 'bg-error-bg text-error',
		muted: 'bg-debug-bg text-muted'
	};
</script>

<span
	class={[
		'inline-flex items-center gap-1.5 rounded-full py-px pr-[9px] pl-[7px] text-xs font-semibold whitespace-nowrap forced-colors:border',
		tones[tone]
	]}
	{title}
>
	<span
		class="size-1.5 shrink-0 rounded-full bg-current forced-color-adjust-none forced-colors:bg-[CanvasText]"
		aria-hidden="true"
	></span>
	{@render children()}
</span>
