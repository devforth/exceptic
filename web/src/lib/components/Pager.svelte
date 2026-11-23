<!--
@component
The navigation under a list: "21–40 of 134" on the left; previous and next, the first and last
pages and two on each side of the current one (filled) in the middle; the page size on the right.
-->
<script lang="ts">
	import { perPageChoices } from '#lib/filters.ts';
	import Select from './Select.svelte';

	type Props = {
		page: number;
		perPage: number;
		total: number;
		/** There are more rows than total (a count that stopped at its limit). */
		more?: boolean;
		/** The address of a page. */
		href: (page: number) => string;
		onperpage: (perPage: number) => void;
	};

	let { page, perPage, total, more = false, href, onperpage }: Props = $props();

	const pages = $derived(Math.max(1, Math.ceil(total / perPage)));
	const first = $derived(Math.min(total, (page - 1) * perPage + 1));
	const last = $derived(Math.min(total, page * perPage));

	// The page numbers shown, 0 for a gap.
	const numbers = $derived.by(() => {
		const shown: number[] = [];

		for (let number = 1; number <= pages; number++) {
			if (number !== 1 && number !== pages && (number < page - 2 || number > page + 2)) continue;
			if (shown.length && number > shown[shown.length - 1] + 1) shown.push(0);
			shown.push(number);
		}

		return shown;
	});

	const perPageOptions = perPageChoices.map((choice) => ({
		value: String(choice),
		label: `${choice} per page`
	}));

	// The boxed links; the current page filled, the ends that lead nowhere faded.
	const box = 'min-w-8 rounded-control border px-2 py-1 text-center';
	const linkClasses = `${box} border-border bg-surface no-underline focus-ring hover:bg-hover hover:no-underline`;
	const disabledClasses = `${box} border-border bg-surface text-muted opacity-60`;
	const currentClasses = `${box} border-accent bg-accent font-semibold text-accent-text`;
</script>

{#if total > 0}
	<nav aria-label="Pages" class="my-3 flex flex-wrap items-center justify-between gap-2">
		<span class="text-xs text-muted">{first}–{last} of {total}{more ? '+' : ''}</span>
		{#if pages > 1}
			<span class="flex flex-wrap gap-1">
				{#if page > 1}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the list's own address -->
					<a href={href(page - 1)} rel="prev" class={linkClasses}>← Prev</a>
				{:else}
					<span class={disabledClasses}>← Prev</span>
				{/if}
				{#each numbers as number, index (index)}
					{#if number === 0}
						<span class="min-w-8 px-2 py-1 text-center text-muted">…</span>
					{:else if number === page}
						<span class={currentClasses} aria-current="page">
							{number}
						</span>
					{:else}
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the list's own address -->
						<a href={href(number)} class={linkClasses}>{number}</a>
					{/if}
				{/each}
				{#if page < pages}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the list's own address -->
					<a href={href(page + 1)} rel="next" class={linkClasses}>Next →</a>
				{:else}
					<span class={disabledClasses}>Next →</span>
				{/if}
			</span>
		{/if}
		<Select
			label="Rows per page"
			value={String(perPage)}
			options={perPageOptions}
			onchange={(value: string) => onperpage(Number(value))}
		/>
	</nav>
{/if}
