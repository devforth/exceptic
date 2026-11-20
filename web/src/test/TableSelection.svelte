<!-- A page with a selectable Table bound to its state, and other rows on demand (Table.test.ts). -->
<script lang="ts">
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';

	type Item = { id: number; name: string };

	const columns: TableColumn<'name'>[] = [{ key: 'name', label: 'Name' }];

	let rows: Item[] = $state([
		{ id: 1, name: 'one' },
		{ id: 2, name: 'two' }
	]);
	let selected: ReadonlySet<string | number> = $state(new Set());
</script>

<Table {columns} {rows} rowKey={(row: Item) => row.id} bind:selected label="Items">
	{#snippet cell(row: Item)}{row.name}{/snippet}
</Table>
<p>Checked: {[...selected].join(', ') || 'none'}</p>
<button type="button" onclick={() => (rows = [...rows])}>Other rows</button>
