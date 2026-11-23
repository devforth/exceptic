<!--
@component
A note in its tone. Information, a warning and an error are tinted callouts with a bar in their
tone at the left; a success is a short message in the info tint, as the toast is. `compact` sets
the text a size smaller (a note beside a form's fields). Paragraphs inside are spaced.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { NoticeTone } from './notice.ts';

	type Props = { tone: NoticeTone; compact?: boolean; children: Snippet };

	let { tone, compact = false, children }: Props = $props();

	const callout =
		'relative rounded-popup py-3 pr-4 pl-5 text-text before:absolute before:inset-y-2 before:left-0 before:w-1 before:rounded-r-[4px] forced-colors:border-l-4 forced-colors:before:hidden';

	const tones: Record<NoticeTone, string> = {
		info: `${callout} bg-info-bg before:bg-info`,
		success: 'rounded-control bg-info-bg px-3 py-2 text-info forced-colors:border',
		warning: `${callout} bg-warning-bg before:bg-warning`,
		error: `${callout} bg-error-bg before:bg-error`
	};
</script>

<div class={['min-w-0 [&_p+p]:mt-1.5', compact && 'text-label', tones[tone]]}>
	{@render children()}
</div>
