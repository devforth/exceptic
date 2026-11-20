<!--
@component
The navigation under a list: "21–40 of 134", previous and next, the first and last pages and
two on each side of the current one, and the page size.
-->
<script lang="ts">
	import { perPageChoices } from '#lib/filters.ts';
	import { controlClasses } from './field.ts';

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

	const linkClasses =
		'focus-ring rounded-control px-2 py-1 no-underline hover:bg-hover hover:no-underline';
</script>

{#if total > 0}
	<nav aria-label="Pages" class="mt-3 flex flex-wrap items-center gap-3 text-sm">
		<span class="text-xs text-muted">{first}–{last} of {total}{more ? '+' : ''}</span>
		{#if pages > 1}
			<span class="flex flex-wrap items-center gap-1">
				{#if page > 1}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the list's own address -->
					<a href={href(page - 1)} rel="prev" class={linkClasses}>← Prev</a>
				{:else}
					<span class="px-2 text-muted">← Prev</span>
				{/if}
				{#each numbers as number, index (index)}
					{#if number === 0}
						<span class="px-1 text-muted">…</span>
					{:else if number === page}
						<span class="rounded-control bg-selected px-2 py-1 font-semibold" aria-current="page">
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
					<span class="px-2 text-muted">Next →</span>
				{/if}
			</span>
		{/if}
		<select
			class={controlClasses}
			aria-label="Rows per page"
			value={perPage}
			onchange={(event) => onperpage(Number(event.currentTarget.value))}
		>
			{#each perPageChoices as choice (choice)}
				<option value={choice}>{choice} per page</option>
			{/each}
		</select>
	</nav>
{/if}
