<!--
@component
Tabs over panels, as a segmented control. One Tab stop: the arrows move between the tabs (and
wrap), Home and End go to the ends (#lib/roving.ts). With `param` the address names the tab
(?param=id), read on load and replaced without a navigation (a shallow goto), so a reload or a
shared link opens the same tab.
-->
<script lang="ts">
	import { untrack, type Snippet } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { movedIndex } from '#lib/roving.ts';
	import type { Tab } from './tabs.ts';

	type Props = {
		/** What the tabs choose, e.g. "MCP client". */
		label: string;
		tabs: Tab[];
		/** The query parameter that names the tab. */
		param?: string;
		selected?: string;
		/** The panel of a tab, by its id. */
		panel: Snippet<[string]>;
	};

	let { label, tabs, param, selected = $bindable(), panel }: Props = $props();

	const prefix = $props.id();

	let list: HTMLElement;

	const shown = $derived(tabs.some((tab) => tab.id === selected) ? selected : tabs[0]?.id);

	// The tab the address names, once: later the tabs change it themselves.
	untrack(() => {
		const named = param && page.url.searchParams.get(param);

		if (named && tabs.some((tab) => tab.id === named)) selected = named;
	});

	function select(id: string) {
		selected = id;

		if (param) {
			const url = new URL(page.url.href);

			url.searchParams.set(param, id);
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- this page with another query
			void goto(url, { shallow: true, replace: true, state: page.state });
		}
	}

	function keydown(event: KeyboardEvent) {
		const index = movedIndex(
			event.key,
			tabs.findIndex((tab) => tab.id === shown),
			tabs.length,
			'horizontal'
		);

		if (index === undefined) return;

		event.preventDefault();
		select(tabs[index].id);
		list.querySelectorAll<HTMLElement>('[role="tab"]')[index]?.focus();
	}
</script>

<div
	bind:this={list}
	role="tablist"
	aria-label={label}
	tabindex="-1"
	class="mb-3 inline-flex flex-wrap gap-0.5 rounded-note bg-hover p-1"
	onkeydown={keydown}
>
	{#each tabs as tab (tab.id)}
		{@const current = tab.id === shown}
		<button
			type="button"
			role="tab"
			id="{prefix}-tab-{tab.id}"
			aria-selected={current}
			aria-controls="{prefix}-panel-{tab.id}"
			tabindex={current ? 0 : -1}
			class={[
				'cursor-pointer rounded-control px-3.5 py-1.5 font-medium focus-ring',
				current
					? 'bg-surface text-text shadow-sm ring-1 ring-border forced-colors:outline-2'
					: 'text-muted hover:text-text'
			]}
			onclick={() => select(tab.id)}
		>
			{tab.label}
		</button>
	{/each}
</div>

{#each tabs as tab (tab.id)}
	<div
		role="tabpanel"
		id="{prefix}-panel-{tab.id}"
		aria-labelledby="{prefix}-tab-{tab.id}"
		tabindex="0"
		class="rounded-control focus-ring"
		hidden={tab.id !== shown}
	>
		{#if tab.id === shown}{@render panel(tab.id)}{/if}
	</div>
{/each}
