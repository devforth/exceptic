<!--
@component
The records one audit log entry changed when its request changed several (a bulk resolve, deleted
logs): what every record changed alike once, as an AuditDiff, then a list of the records with their
project and a column for each field that differs between them (an issue's status before,
in red as every value before a change, its row tinted as the issue list tints it). A record
that still exists links to its page (`href`).
-->
<script lang="ts">
	import {
		groupFields,
		itemValue,
		searchParts,
		type AuditItemFields,
		type ItemField
	} from '#lib/audit.ts';
	import AuditDiff from './AuditDiff.svelte';
	import Badge from './Badge.svelte';
	import AuditValue from './AuditValue.svelte';
	import Button from './Button.svelte';
	import Table from './Table.svelte';
	import { statusRowTone } from './issueStatus.ts';
	import type { TableColumn } from './table.ts';

	type Item = AuditItemFields & { record_id: string; project?: string; href?: string };

	/** query: the list's search; the records it finds come first, the text it found marked. */
	type Props = { items: Item[]; query?: string };

	let { items, query = '' }: Props = $props();

	const found = (item: Item) =>
		searchParts(item.record_name || item.record_id, query).some((part) => part.changed);
	const rows = $derived([...items.filter(found), ...items.filter((item) => !found(item))]);

	const grouped = $derived(groupFields(items));
	const hasCommon = $derived(
		Object.keys(grouped.common.before).length + Object.keys(grouped.common.after).length > 0
	);

	type ItemColumn = TableColumn<string>;

	const columns: ItemColumn[] = $derived([
		{ key: 'project', label: 'Project' },
		{ key: 'record', label: 'Record', fill: true },
		...grouped.varying.map((field) => ({ key: `${field.side}:${field.name}`, label: field.label }))
	]);
	// A record's row is tinted by its status before the change, as the issue list tints it.
	const statusField = $derived(
		grouped.varying.find((field) => field.side === 'before' && field.name === 'status')
	);
	const fieldsByKey = $derived(
		new Map<string, ItemField>(
			grouped.varying.map((field) => [`${field.side}:${field.name}`, field])
		)
	);
</script>

{#if hasCommon}
	<AuditDiff before={grouped.common.before} after={grouped.common.after} />
{/if}

{#snippet name(row: Item)}
	{#each searchParts(row.record_name || row.record_id, query) as part, index (index)}{#if part.changed}<mark
				class="rounded-xs bg-warning/30 text-inherit"
			>
				{part.text}
			</mark>{:else}{part.text}{/if}{/each}
{/snippet}

<Table
	{columns}
	{rows}
	rowTone={(row: Item) => (statusField ? statusRowTone(itemValue(row, statusField)) : undefined)}
	label="Changed records"
	rowKey={(row: Item) => row.record_id}
	compact
	flat
>
	{#snippet cell(row: Item, column: ItemColumn)}
		{#if column.key === 'record'}
			{#if row.href}
				<Button variant="title" truncate title={row.record_name} href={row.href}>
					{@render name(row)}
				</Button>
			{:else}
				<span class="block truncate" title={row.record_name}>{@render name(row)}</span>
			{/if}
		{:else if column.key === 'project'}
			{#if row.project}<Badge tone="project" label={row.project} />{/if}
		{:else}
			{@const field = fieldsByKey.get(column.key)}
			{#if field}
				<AuditValue side={field.side} parts={[{ text: itemValue(row, field), changed: false }]} />
			{/if}
		{/if}
	{/snippet}
	{#snippet empty()}No record you can see.{/snippet}
</Table>
