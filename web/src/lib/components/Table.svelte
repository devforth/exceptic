<!--
@component
A table of rows: the columns name what each shows, the cell snippet draws a row's cell.
Every second row is shaded. Without rows it shows the empty snippet in an EmptyState.
- `scroll` keeps it in a box of its own height with the header in view; `flat` has no box of
  its own (inside a Card), only scrolls sideways; `compact` is denser.
- `sort`: columns with a `sort` link their heading to the list sorted by them (aria-sort).
- `selected` (bindable, needs rowKey): a checkbox per row and one for all (mixed when some are
  checked); it empties when the rows change.
- `rowTone` tints a row (error, success, faded) and `rowTitle` is its tooltip; `selectedKey`
  marks the row shown elsewhere on the page; `headerless` keeps the headings for screen
  readers only.
-->
<script lang="ts" generics="Row, Key extends string">
	import type { Snippet } from 'svelte';
	import EmptyState from './EmptyState.svelte';
	import { checkClasses } from './field.ts';
	import Icon from './Icon.svelte';
	import {
		columnSort,
		type RowTone,
		type SortDirection,
		type TableColumn,
		type TableSort
	} from './table.ts';

	type RowKey = string | number;

	type Props = {
		columns: TableColumn<Key>[];
		rows: Row[];
		cell: Snippet<[Row, TableColumn<Key>]>;
		empty?: Snippet;
		/** Its accessible name. */
		label?: string;
		rowKey?: (row: Row) => RowKey;
		compact?: boolean;
		scroll?: boolean;
		flat?: boolean;
		headerless?: boolean;
		sort?: TableSort;
		selected?: ReadonlySet<RowKey>;
		/** The name of a row's checkbox, e.g. "Select TypeError: x is undefined". */
		selectLabel?: (row: Row) => string;
		rowTone?: (row: Row) => RowTone | undefined;
		rowTitle?: (row: Row) => string | undefined;
		selectedKey?: RowKey;
	};

	let {
		columns,
		rows,
		cell,
		empty,
		label,
		rowKey,
		compact = false,
		scroll = false,
		flat = false,
		headerless = false,
		sort,
		selected = $bindable(),
		selectLabel,
		rowTone,
		rowTitle,
		selectedKey
	}: Props = $props();

	const box = $derived(
		scroll
			? 'max-h-[420px] overflow-auto rounded-box border border-border bg-surface focus-ring'
			: flat
				? 'overflow-x-auto'
				: 'overflow-x-auto rounded-card border border-transparent bg-surface shadow-card'
	);

	const padding = $derived(compact ? 'px-2.5 py-1.5' : 'px-3 py-2.5');

	function columnClasses(column: TableColumn<Key>) {
		return [
			padding,
			'text-left align-top',
			column.number && 'text-right whitespace-nowrap tabular-nums',
			column.actions && 'w-[1%] whitespace-nowrap',
			column.fill && 'w-full max-w-0',
			column.wideOnly && 'max-table:hidden'
		];
	}

	const headingClasses = $derived([
		'border-b border-border text-xs font-medium tracking-[.03em] text-muted uppercase',
		scroll && 'sticky top-0 z-1 bg-surface shadow-[inset_0_-1px_0_var(--color-border)]'
	]);

	// Literal class names, so Tailwind builds them.
	const tones: Record<RowTone, string> = {
		error: '[&>*]:bg-row-error even:[&>*]:bg-row-error-even',
		success: '[&>*]:bg-row-success even:[&>*]:bg-row-success-even',
		faded: 'opacity-50 even:[&>*]:bg-[linear-gradient(var(--color-stripe),var(--color-stripe))]'
	};
	const striped = 'even:[&>*]:bg-[linear-gradient(var(--color-stripe),var(--color-stripe))]';

	function rowClasses(row: Row) {
		const tone = rowTone?.(row);

		return [
			'border-b border-border last:border-b-0',
			selectedKey !== undefined && rowKey?.(row) === selectedKey
				? '[&>*]:bg-selected'
				: tone
					? tones[tone]
					: striped
		];
	}

	const sortIcons = {
		ascending: 'sortedAscending',
		descending: 'sortedDescending',
		none: 'sortable'
	} as const satisfies Record<SortDirection | 'none', string>;

	// Selection: every row's key, and how many of them are checked.
	const selectable = $derived(selected !== undefined && rowKey !== undefined);
	const keys = $derived(rowKey ? rows.map(rowKey) : []);
	const checkedCount = $derived(keys.filter((key) => selected?.has(key)).length);

	// A new set each time: the bound value changes, so the page sees it.
	function check(key: RowKey, checked: boolean) {
		const others = [...(selected ?? [])].filter((item) => item !== key);

		selected = new Set(checked ? [...others, key] : others);
	}

	// Other rows (another page, a filter): what was checked is no longer in view.
	let checkedRows: Row[] | undefined;

	$effect(() => {
		if (checkedRows !== undefined && rows !== checkedRows && selected?.size) {
			selected = new Set();
		}

		checkedRows = rows;
	});
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
			<thead class={headerless ? 'sr-only' : undefined}>
				<tr>
					{#if selectable}
						<th scope="col" class={[padding, headingClasses, 'w-[1%] pr-0']}>
							<input
								type="checkbox"
								class={checkClasses.checkbox}
								aria-label="Select all"
								checked={keys.length > 0 && checkedCount === keys.length}
								{@attach (box: HTMLInputElement) => {
									box.indeterminate = checkedCount > 0 && checkedCount < keys.length;
								}}
								onchange={(event) =>
									(selected = event.currentTarget.checked ? new Set(keys) : new Set())}
							/>
						</th>
					{/if}
					{#each columns as column (column.key)}
						{@const state = column.sort && sort ? columnSort(column.sort, sort.current) : undefined}
						<th
							scope="col"
							aria-sort={state?.direction}
							class={[columnClasses(column), headingClasses, state?.direction && 'text-text']}
						>
							{#if state && sort}
								<!-- eslint-disable svelte/no-navigation-without-resolve -- the list's own address -->
								<a
									href={sort.href(state.next)}
									title={state.direction ? 'Reverse the order' : `Sort by ${column.label}`}
									class={[
										'inline-flex items-center gap-1 rounded-control text-inherit no-underline focus-ring hover:text-text hover:no-underline',
										column.number && 'flex-row-reverse'
									]}
								>
									{column.label}
									<!-- Unsorted: dim, as both arrows are; sorted: its direction, strong. -->
									<span class={['flex', !state.direction && 'opacity-30']}>
										<Icon name={sortIcons[state.direction ?? 'none']} size={14} />
									</span>
								</a>
								<!-- eslint-enable svelte/no-navigation-without-resolve -->
							{:else}
								<!-- The buttons need no heading to be seen; screen readers still get one. -->
								<span class={column.actions ? 'sr-only' : undefined}>{column.label}</span>
							{/if}
						</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each rows as row, index (rowKey ? rowKey(row) : index)}
					{@const key = rowKey?.(row)}
					<tr
						class={rowClasses(row)}
						title={rowTitle?.(row)}
						aria-current={selectedKey !== undefined && key === selectedKey ? 'true' : undefined}
					>
						{#if selectable && key !== undefined}
							<td class={[padding, 'w-[1%] pr-0 align-top']}>
								<input
									type="checkbox"
									class={checkClasses.checkbox}
									aria-label={selectLabel?.(row) ?? 'Select'}
									checked={selected?.has(key)}
									onchange={(event) => check(key, event.currentTarget.checked)}
								/>
							</td>
						{/if}
						{#each columns as column (column.key)}
							{#if column.rowHeader}
								<th
									scope="row"
									class={[columnClasses(column), 'w-[30%] font-normal break-all text-muted']}
								>
									{@render cell(row, column)}
								</th>
							{:else}
								<td class={[columnClasses(column), 'wrap-break-word']}>
									{@render cell(row, column)}
								</td>
							{/if}
						{/each}
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
