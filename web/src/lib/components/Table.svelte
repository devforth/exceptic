<!--
@component
A table of rows: the columns name what each shows, the cell snippet draws a row's cell.
Every second row is shaded. Without rows it shows the empty snippet in an EmptyState.
`scroll` keeps it in a box of its own height with the header in view.
-->
<script lang="ts" generics="Row, Key extends string">
	import type { Snippet } from 'svelte';
	import type { TableColumn } from './table.ts';
	import EmptyState from './EmptyState.svelte';

	type Props = {
		columns: TableColumn<Key>[];
		rows: Row[];
		cell: Snippet<[Row, TableColumn<Key>]>;
		empty?: Snippet;
		/** Its accessible name. */
		label?: string;
		rowKey?: (row: Row) => string | number;
		compact?: boolean;
		scroll?: boolean;
	};

	let {
		columns,
		rows,
		cell,
		empty,
		label,
		rowKey,
		compact = false,
		scroll = false
	}: Props = $props();

	const box = $derived(
		scroll
			? 'max-h-[420px] overflow-auto rounded-box border border-border bg-surface focus-ring'
			: 'overflow-x-auto rounded-card border border-transparent bg-surface shadow-card'
	);

	const padding = $derived(compact ? 'px-2.5 py-1.5' : 'px-3 py-2.5');

	function columnClasses(column: TableColumn<Key>) {
		return [
			padding,
			'text-left align-top',
			column.number && 'text-right whitespace-nowrap tabular-nums',
			column.actions && 'w-[1%] whitespace-nowrap',
			column.wideOnly && 'max-table:hidden'
		];
	}
</script>

{#if rows.length === 0 && empty}
	<EmptyState>{@render empty()}</EmptyState>
{:else}
	<!-- A scroll box takes focus, so the keyboard can scroll a table without focusable cells. -->
	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<div
		class={box}
		tabindex={scroll ? 0 : undefined}
		role={scroll ? 'region' : undefined}
		aria-label={scroll ? label : undefined}
	>
		<table class="w-full border-collapse" aria-label={label}>
			<thead>
				<tr>
					{#each columns as column (column.key)}
						<th
							scope="col"
							class={[
								columnClasses(column),
								'border-b border-border text-xs font-medium tracking-[.03em] text-muted uppercase',
								scroll && 'sticky top-0 z-1 bg-surface shadow-[inset_0_-1px_0_var(--color-border)]'
							]}
						>
							<!-- The buttons need no heading to be seen; screen readers still get one. -->
							<span class={column.actions ? 'sr-only' : undefined}>{column.label}</span>
						</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each rows as row, index (rowKey ? rowKey(row) : index)}
					<tr
						class="border-b border-border last:border-b-0 even:[&>td]:bg-[linear-gradient(var(--color-stripe),var(--color-stripe))]"
					>
						{#each columns as column (column.key)}
							<td class={[columnClasses(column), 'wrap-break-word']}>
								{@render cell(row, column)}
							</td>
						{/each}
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
