<!--
@component
A list filter of several values: a dropdown of checkboxes whose button says "All projects"
or "Projects: 2". The option snippet draws a value (a badge). Hidden when there is nothing
to choose (one option and none chosen).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { controlClasses } from './field.ts';
	import Icon from './Icon.svelte';

	type Props = {
		/** What the values are, e.g. "Projects". */
		label: string;
		/** The button without a choice, e.g. "All projects". */
		all: string;
		options: readonly string[];
		selected: readonly string[];
		onchange: (selected: string[]) => void;
		option: Snippet<[string]>;
	};

	let { label, all, options, selected, onchange, option }: Props = $props();

	let details: HTMLDetailsElement | undefined = $state();

	// A click elsewhere or Esc closes it, as a menu does.
	$effect(() => {
		const closeOutside = (event: MouseEvent) => {
			if (details?.open && !details.contains(event.target as Node)) details.open = false;
		};
		const closeOnEscape = (event: KeyboardEvent) => {
			if (details?.open && event.key === 'Escape') {
				details.open = false;
				details.querySelector('summary')?.focus();
			}
		};

		document.addEventListener('click', closeOutside);
		document.addEventListener('keydown', closeOnEscape);

		return () => {
			document.removeEventListener('click', closeOutside);
			document.removeEventListener('keydown', closeOnEscape);
		};
	});

	function toggle(value: string, checked: boolean) {
		const next = checked ? [...selected, value] : selected.filter((chosen) => chosen !== value);

		onchange(options.filter((known) => next.includes(known)));
	}

	const optionClasses =
		'flex cursor-pointer items-center gap-2 rounded-control px-2 py-1.5 whitespace-nowrap hover:bg-hover';
</script>

{#if options.length > 1 || selected.length > 0}
	<details bind:this={details} class="relative">
		<summary
			class={[controlClasses, 'flex cursor-pointer list-none items-center gap-1.5 marker:hidden']}
		>
			{selected.length ? `${label}: ${selected.length}` : all}
			<Icon name="expand" size={16} />
		</summary>
		<div
			class="absolute z-20 mt-1 min-w-48 rounded-popup border border-border bg-surface p-1 shadow-pop"
		>
			<label class={optionClasses}>
				<input type="checkbox" checked={selected.length === 0} onchange={() => onchange([])} />
				{all}
			</label>
			{#each options as value (value)}
				<label class={optionClasses}>
					<input
						type="checkbox"
						checked={selected.includes(value)}
						onchange={(event) => toggle(value, event.currentTarget.checked)}
					/>
					{@render option(value)}
				</label>
			{/each}
		</div>
	</details>
{/if}
