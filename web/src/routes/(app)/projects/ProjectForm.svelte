<!--
@component
The form of a project, new or saved: its name, color, DSN and the levels it records. It sends the whole form through save; a refused field
shows its problem beside it.
-->
<script lang="ts">
	import ProjectNameField from './ProjectNameField.svelte';
	import { resolve } from '$app/paths';
	import { help } from '#lib/api/catalog.gen.ts';
	import Button from '#lib/components/Button.svelte';
	import CopyField from '#lib/components/CopyField.svelte';
	import Details from '#lib/components/Details.svelte';
	import Hint from '#lib/components/Hint.svelte';
	import LevelChoices from '#lib/components/LevelChoices.svelte';
	import LevelSummary from '#lib/components/LevelSummary.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import { FormState } from '#lib/form.svelte.ts';
	import type { ProjectFormValues } from '#lib/projects.ts';
	import ColorField from './ColorField.svelte';

	type Props = {
		/** What the form starts with. */
		initial: ProjectFormValues;
		/** Set by PROJECTS: its key comes from there and it cannot be deleted here. */
		fromEnv?: boolean;
		/** A saved project's DSN; none for a new one. */
		dsn?: string;
		/** The button that sends it: "Add" or "Save". */
		submitLabel: string;
		save: (values: ProjectFormValues) => Promise<unknown>;
	};

	let { initial, fromEnv = false, dsn, submitLabel, save }: Props = $props();

	// The form keeps what the user changed; it starts from initial once.
	// svelte-ignore state_referenced_locally
	let values: ProjectFormValues = $state({ ...initial });

	const form = new FormState();
	const projectHelp = help.fields.Projects;

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		await form.submit(() => save($state.snapshot(values)));
	}
</script>

<form class="flex max-w-[720px] flex-col gap-5" onsubmit={submit}>
	{#if fromEnv}
		<Notice tone="info">
			Added by PROJECTS: its DSN key comes from there, and it can be deleted only after it is
			removed from there.
		</Notice>
	{/if}
	<ProjectNameField bind:value={values.name} error={form.fields.name} />

	<ColorField
		bind:value={values.color}
		name={values.name}
		hint={projectHelp.Color}
		error={form.fields.color}
	/>

	<div class="flex flex-col gap-1">
		<span class="flex items-center gap-1.5 text-label text-muted">
			DSN <Hint text={projectHelp.DSN} />
		</span>
		{#if dsn}
			<CopyField
				value={dsn}
				label="DSN of {values.name}"
				copyTitle="Copy the DSN"
				copied="DSN copied to clipboard."
			/>
		{:else}
			<p class="text-sm text-muted">A DSN key is generated when the project is added.</p>
		{/if}
	</div>

	<Details
		title="Levels to record"
		variant="form"
		invalid={Boolean(form.fields.event_levels || form.fields.log_levels)}
	>
		{#snippet summary()}
			<span class="flex flex-wrap items-center gap-1.5">
				<span class="text-muted">Errors</span>
				<LevelSummary kind="event" chosen={values.eventLevels} />
			</span>
			<span class="flex flex-wrap items-center gap-1.5">
				<span class="text-muted">Logs</span>
				<LevelSummary kind="log" chosen={values.logLevels} />
			</span>
		{/snippet}
		<p class="text-xs text-muted">
			Errors and logs of levels left unchecked are dropped when they arrive: not stored, no
			notifications.
		</p>
		<LevelChoices
			kind="event"
			legend="Errors and messages"
			hint={projectHelp['Errors and messages']}
			selected={values.eventLevels}
			onchange={(selected: string[]) => (values.eventLevels = selected)}
			error={form.fields.event_levels}
		/>
		<LevelChoices
			kind="log"
			legend="Logs"
			hint={projectHelp.Logs}
			selected={values.logLevels}
			onchange={(selected: string[]) => (values.logLevels = selected)}
			error={form.fields.log_levels}
		/>
	</Details>

	{#if form.error}<Notice tone="error">{form.error}</Notice>{/if}

	<div class="flex flex-wrap gap-2">
		<Button variant="primary" type="submit" busy={form.busy}>{submitLabel}</Button>
		<Button href={resolve('/(app)/projects')}>Cancel</Button>
	</div>
</form>
