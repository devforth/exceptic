<!--
@component
A part that opens and closes, its heading the toggle.
- `section`: a card with an h2, a count beside it and a detail cut to one line (summary); the
  sections of the issue page.
- `form`: a card in a form with its title, Hint, and what is set inside on the line under it
  (summary); it opens when a value inside is refused (invalid).
- `disclosure`: a card of optional settings: its title, a line under it on what is inside
  (summary), and a body split from it by a line (Advanced settings); FormGroup parts the body.
- `nested`: a flat bordered box with its title and count, its body set off by a line on the
  left (a stack trace inside an exception). A `section` inside another Details is flat too.
- `inline`: a small disclosure inside other content (a context, local variables).
`open` starts it open.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Hint from './Hint.svelte';
	import Icon from './Icon.svelte';
	import { boxClasses, cardClasses } from './surface.ts';

	type Props = {
		title: string;
		variant?: 'section' | 'nested' | 'form' | 'disclosure' | 'inline';
		/** section, nested: the number of rows inside, beside the title. */
		count?: number;
		/** What the count counts, after it ("10 frames"). */
		unit?: string;
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
		unit,
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
	const nested = $derived(variant === 'nested');

	const countClasses =
		'rounded-full bg-[color-mix(in_srgb,var(--color-text)_8%,var(--color-surface))] px-2 text-xs leading-5 font-semibold';
</script>

<details
	bind:open
	class={[
		nested
			? ['mt-2', boxClasses]
			: card
				? [cardClasses, 'in-[details]:border in-[details]:border-border in-[details]:shadow-none']
				: 'my-1 text-sm',
		variant === 'disclosure' &&
			'overflow-hidden [interpolate-size:allow-keywords] details-content:h-0 details-content:overflow-hidden details-content:transition-[height,content-visibility] details-content:transition-discrete details-content:duration-250 open:details-content:h-auto motion-reduce:details-content:transition-none'
	]}
>
	<summary
		class={[
			'cursor-pointer list-none marker:hidden [&::-webkit-details-marker]:hidden',
			card
				? [
						'relative flex min-w-0 flex-wrap items-center pr-[58px] pl-5 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent',
						nested ? 'rounded-box py-2.5' : 'rounded-card py-3.5',
						variant === 'form' ? 'gap-x-2 gap-y-1.5' : 'gap-x-2.5 gap-y-1'
					]
				: 'inline-flex items-center gap-1.5 rounded-control focus-ring'
		]}
	>
		{#if variant === 'inline'}
			<span
				class="opacity-70 transition-[rotate] motion-reduce:transition-none [details[open]>summary>&]:rotate-90"
			>
				<Icon name="chevronRight" size={14} />
			</span>
			{title}
		{:else}
			{#if variant === 'section'}
				<h2 class="text-md font-bold">{title}</h2>
				{#if count !== undefined}<span class={countClasses}>
						{count}{unit ? ` ${unit}` : ''}
					</span>{/if}
				{#if summary}
					<span class="min-w-0 flex-1 truncate text-xs text-muted">{@render summary()}</span>
				{/if}
			{:else if nested}
				<span class="font-semibold">{title}</span>
				{#if count !== undefined}<span class={countClasses}>
						{count}{unit ? ` ${unit}` : ''}
					</span>{/if}
			{:else if variant === 'disclosure'}
				<span class="flex basis-full flex-col gap-0.5 py-0.5">
					<span class="text-md font-semibold">{title}</span>
					{#if summary}<span class="text-label text-muted">{@render summary()}</span>{/if}
				</span>
			{:else}
				<span class="font-semibold">{title}</span>
				{#if hint}<Hint text={hint} />{/if}
				{#if summary}
					<span class="flex basis-full flex-wrap items-center gap-1 text-xs">
						{@render summary()}
					</span>
				{/if}
			{/if}
			<span
				class="absolute {nested
					? 'top-[7px]'
					: 'top-[13px]'} right-3.5 flex size-[30px] items-center justify-center rounded-full bg-hover text-muted transition-[rotate,background-color] duration-200 motion-reduce:transition-none forced-colors:border [details[open]>summary>&]:rotate-180 [summary:hover>&]:bg-[color-mix(in_srgb,var(--color-text)_12%,var(--color-surface))]"
				aria-hidden="true"
			>
				<Icon name="expand" size={16} />
			</span>
		{/if}
	</summary>
	<div
		class={variant === 'disclosure'
			? 'flex flex-col gap-5.5 border-t border-border p-4.5'
			: nested
				? 'mr-3.5 mb-3 ml-5 border-l-2 border-border pl-3.5'
				: card
					? 'flex flex-col gap-2 px-5 pb-5'
					: 'mt-1.5'}
	>
		{@render children()}
	</div>
</details>
