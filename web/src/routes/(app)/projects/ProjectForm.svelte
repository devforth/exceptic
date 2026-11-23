<!--
@component
The settings page of a saved project: its title (renamed in place by the pencil), color, the
levels it records and its disk share. Save sends all of it at once; a refused field shows its
problem beside it. `actions` go in the heading, `intro` under it.
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import { diskShare, help } from '#lib/api/catalog.gen.ts';
	import type { Out } from '#lib/api/client.ts';
	import type { Snippet } from 'svelte';
	import Button from '#lib/components/Button.svelte';
	import ColorField from '#lib/components/ColorField.svelte';
	import Details from '#lib/components/Details.svelte';
	import EditableTitle from '#lib/components/EditableTitle.svelte';
	import LevelSummary from '#lib/components/LevelSummary.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import { FormState } from '#lib/form.svelte.ts';
	import { formatSize } from '#lib/format.ts';
	import { projectNameRules, type ProjectFormValues } from '#lib/projects.ts';
	import DiskShareField from './DiskShareField.svelte';
	import LogLevelsField from './LogLevelsField.svelte';

	type Props = {
		id: number;
		name: string;
		/** What the form starts with. */
		initial: ProjectFormValues;
		storage: Out<'get_storage'>;
		save: (values: ProjectFormValues, name: string) => Promise<unknown>;
		actions?: Snippet;
		intro?: Snippet;
	};

	let { id, name, initial, storage, save, actions, intro }: Props = $props();

	// The form keeps what the user changed; it starts from initial once.
	// svelte-ignore state_referenced_locally
	let values: ProjectFormValues = $state({ ...initial });
	// svelte-ignore state_referenced_locally
	let newName = $state(name);

	const form = new FormState();
	const budget = $derived(storage.limit_bytes ?? 0);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		await form.submit(() => save($state.snapshot(values), newName.trim()));
	}
</script>

<form class="flex flex-col gap-3" onsubmit={submit}>
	<div>
		<PageHeading title={name} {actions}>
			{#snippet heading()}
				<EditableTitle
					title={name}
					bind:value={newName}
					label="Project name"
					{...projectNameRules}
					error={form.fields.name}
				/>
			{/snippet}
		</PageHeading>
		{@render intro?.()}
	</div>
	<ColorField
		bind:value={values.color}
		name={newName}
		hint={help.fields.Projects.Color}
		error={form.fields.color}
	/>

	<p>
		<Button icon="code" href={resolve('/(app)/projects/[id=projectId]/setup', { id })}>
			Show installation code
		</Button>
	</p>

	<Details title="Logs to record" variant="form" invalid={Boolean(form.fields.log_levels)}>
		{#snippet summary()}
			<LevelSummary kind="log" chosen={values.logLevels} />
		{/snippet}
		<LogLevelsField bind:value={values.logLevels} error={form.fields.log_levels} />
	</Details>

	<Details title="Disk share" variant="form" open invalid={Boolean(form.fields.disk_share)}>
		{#snippet summary()}
			<span class="text-muted">
				{#if values.diskShare >= diskShare.max}
					no limit of its own
				{:else if budget}
					up to {values.diskShare}% · {formatSize((budget * values.diskShare) / 100)}
				{:else}
					up to {values.diskShare}%
				{/if}
			</span>
		{/snippet}
		<DiskShareField
			bind:value={values.diskShare}
			{storage}
			projectId={id}
			error={form.fields.disk_share}
		/>
	</Details>

	{#if form.error}<Notice tone="error">{form.error}</Notice>{/if}

	<div class="flex flex-wrap gap-2">
		<Button variant="primary" type="submit" busy={form.busy}>Save</Button>
		<Button href={resolve('/(app)/projects')}>Cancel</Button>
	</div>
</form>
