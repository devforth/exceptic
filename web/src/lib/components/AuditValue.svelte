<!--
@component
A value of an audit log entry as a line of a diff: before a change in red after a minus, after it
in green after a plus, the part that changed (`parts`) a shade deeper; monospace, wrapped.
-->
<script lang="ts">
	import { sideClasses, type ValuePart } from '#lib/audit.ts';

	type Props = { side: 'before' | 'after'; parts: ValuePart[] };

	let { side, parts }: Props = $props();
</script>

<span
	class={['rounded-sm flex gap-2 px-1.5 py-0.5 font-mono text-xs', sideClasses[side].line]}
	data-side={side}
>
	<span class="shrink-0 select-none" aria-hidden="true">{side === 'before' ? '−' : '+'}</span>
	<span class="sr-only">{side === 'before' ? 'Before:' : 'After:'}</span>
	<span class="min-w-0 wrap-break-word whitespace-pre-wrap">
		{#each parts as part, index (index)}{#if part.changed}<mark
					class={['rounded-xs text-inherit', sideClasses[side].changed]}
				>
					{part.text}
				</mark>{:else}{part.text}{/if}{/each}
	</span>
</span>
