<!--
@component
A part that opens and closes, its heading the toggle.
- `section`: a card with an h2, a count beside it and a detail cut to one line (summary); the
  sections of the issue page.
- `form`: a card in a form with its title, Hint, and what is set inside on the line under it
  (summary); it opens when a value inside is refused (invalid).
- `disclosure`: a card of optional settings: its title, a line under it on what is inside
  (summary), and a body split from it by a line (Advanced settings); FormGroup parts the body.
- `inline`: a small disclosure inside other content (a stack trace, a context, local variables).
`open` starts it open.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Hint from './Hint.svelte';
	import Icon from './Icon.svelte';
	import { cardClasses } from './surface.ts';

	type Props = {
		title: string;
		variant?: 'section' | 'form' | 'disclosure' | 'inline';
		/** section: the number of rows inside, beside the title. */
		count?: number;
		/** form: the help text of the title. */
		hint?: string;
		/** section: a detail after the title; form: the values set inside; disclosure: what is inside. */
		summary?: Snippet;
		/** form: a value inside was refused, so it opens. */
		invalid?: boolean;
		open?: boolean;
		children: Snippet;
	};

	let {
		title,
		variant = 'section',
		count,
		hint,
		summary,
		invalid = false,
		open: startOpen = false,
		children
	}: Props = $props();

	// It starts as asked, then the user opens and closes it.
	// svelte-ignore state_referenced_locally
	let open = $state(startOpen);

	$effect(() => {
		if (invalid) open = true;
	});

	const card = $derived(variant !== 'inline');
</script>

<details bind:open class={card ? cardClasses : 'my-1 text-sm'}>
	<summary
		class={[
			'cursor-pointer list-none marker:hidden [&::-webkit-details-marker]:hidden',
			card
				? 'relative flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-1 rounded-card py-3.5 pr-14 pl-5 focus-ring [--focus-offset:-2px]'
				: 'inline-flex items-center gap-1.5 rounded-control focus-ring'
		]}
	>
		{#if variant === 'inline'}
			<span
				class="text-muted transition-[rotate] motion-reduce:transition-none [details[open]>summary>&]:rotate-90"
			>
				<Icon name="chevronRight" size={14} />
			</span>
			{title}
		{:else}
			{#if variant === 'section'}
				<h2 class="text-md font-bold">{title}</h2>
				{#if count !== undefined}
					<span class="rounded-full bg-hover px-2 text-xs leading-5 font-semibold">{count}</span>
				{/if}
				{#if summary}
					<span class="min-w-0 flex-1 truncate text-xs text-muted">{@render summary()}</span>
				{/if}
			{:else if variant === 'disclosure'}
				<span class="flex basis-full flex-col gap-0.5 py-0.5">
					<span class="text-md font-semibold">{title}</span>
					{#if summary}<span class="text-label text-muted">{@render summary()}</span>{/if}
				</span>
			{:else}
				<span class="flex items-center gap-1.5 font-semibold">
					{title}
					{#if hint}<Hint text={hint} />{/if}
				</span>
				{#if summary}
					<span class="flex basis-full flex-wrap items-center gap-1 text-xs">
						{@render summary()}
					</span>
				{/if}
			{/if}
			<span
				class="absolute top-3 right-3.5 flex size-[30px] items-center justify-center rounded-full bg-hover text-muted transition-[rotate] motion-reduce:transition-none [details[open]>summary>&]:rotate-180"
				aria-hidden="true"
			>
				<Icon name="expand" size={16} />
			</span>
		{/if}
	</summary>
	<div
		class={variant === 'disclosure'
			? 'flex flex-col gap-5.5 border-t border-border p-4.5'
			: card
				? 'flex flex-col gap-2 px-5 pb-5'
				: 'mt-1 pl-5'}
	>
		{@render children()}
	</div>
</details>
