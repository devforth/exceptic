<!--
@component
The fields an audit log entry changed, as a diff: each field with its value before (− in red) over
its value after (+ in green), the part that changed a shade deeper. A create has only the after
lines, a delete only the before ones; a change of no field says so.
-->
<script lang="ts">
	import { changedParts, diffRows, type DiffRow } from '#lib/audit.ts';
	import AuditValue from './AuditValue.svelte';
	import Table from './Table.svelte';
	import type { TableColumn } from './table.ts';

	type Props = {
		before?: Record<string, unknown> | null;
		after?: Record<string, unknown> | null;
	};

	let { before, after }: Props = $props();

	type DiffColumn = TableColumn<'field' | 'change'>;

	const rows = $derived(diffRows(before, after));
	const columns: DiffColumn[] = [
		{ key: 'field', label: 'Field', rowHeader: true },
		{ key: 'change', label: 'Change', fill: true }
	];
</script>

<Table
	{columns}
	{rows}
	label="Changed fields"
	rowKey={(row: DiffRow) => row.field}
	compact
	flat
	top
>
	{#snippet cell(row: DiffRow, column: DiffColumn)}
		{#if column.key === 'field'}
			{row.field}
		{:else}
			{@const parts = changedParts(row.before, row.after)}
			<span class="flex flex-col gap-0.5">
				{#if row.before !== undefined}<AuditValue side="before" parts={parts.before} />{/if}
				{#if row.after !== undefined}<AuditValue side="after" parts={parts.after} />{/if}
			</span>
		{/if}
	{/snippet}
	{#snippet empty()}No field changed.{/snippet}
</Table>
