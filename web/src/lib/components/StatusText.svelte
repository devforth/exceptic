<!--
@component
A state as a colored word: done or sent (success), running or retrying (warning), failed
(error), or nothing to say (muted).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { StatusTone } from './status.ts';

	type Props = { tone: StatusTone; children: Snippet };

	let { tone, children }: Props = $props();

	const tones: Record<StatusTone, string> = {
		success: 'text-success',
		warning: 'text-warning',
		error: 'text-error',
		muted: 'text-muted'
	};
</script>

<span class={['text-xs font-semibold whitespace-nowrap', tones[tone]]}>
	{@render children()}
</span>
