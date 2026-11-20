<!--
@component
A state as a colored word: done or sent (success), running or retrying (warning), failed
(error), or nothing to say (muted); title tells more.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { StatusTone } from './status.ts';

	type Props = { tone: StatusTone; title?: string; children: Snippet };

	let { tone, title, children }: Props = $props();

	const tones: Record<StatusTone, string> = {
		success: 'text-success',
		warning: 'text-warning',
		error: 'text-error',
		muted: 'text-muted'
	};
</script>

<span class={['text-xs font-semibold whitespace-nowrap', tones[tone]]} {title}>
	{@render children()}
</span>
