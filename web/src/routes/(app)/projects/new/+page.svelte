<script lang="ts">
	import { goto, invalidate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { help } from '#lib/api/catalog.gen.ts';
	import { post } from '#lib/api/client.ts';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import ColorField from '#lib/components/ColorField.svelte';
	import Details from '#lib/components/Details.svelte';
	import FormActions from '#lib/components/FormActions.svelte';
	import FormGroup from '#lib/components/FormGroup.svelte';
	import PageTitle from '#lib/components/PageTitle.svelte';
	import { FormState } from '#lib/form.svelte.ts';
	import { freeColor, projectInput, type ProjectFormValues } from '#lib/projects.ts';
	import DiskShareField from '../DiskShareField.svelte';
	import LogLevelsField from '../LogLevelsField.svelte';
	import ProjectNameField from '../ProjectNameField.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	// The form starts from the defaults once; a color no project shows yet stands out.
	// svelte-ignore state_referenced_locally
	const initial: ProjectFormValues = {
		color: freeColor(Object.values(data.session?.project_colors ?? {}), data.defaults.shown_color),
		logLevels: data.defaults.log_levels ?? [],
		diskShare: data.defaults.disk_share
	};

	let name = $state('');
	let values: ProjectFormValues = $state({ ...initial });

	const form = new FormState();
	const advancedInvalid = $derived(
		Boolean(form.fields.color || form.fields.log_levels || form.fields.disk_share)
	);

	// The new project's installation page shows its DSN and the code for its SDK.
	async function submit(event: SubmitEvent) {
		event.preventDefault();

		let id = 0;
		const added = await form.submit(async () => {
			const project = await post('create_project', {
				name: name.trim(),
				...projectInput($state.snapshot(values))
			});

			id = project.id;
		});

		if (!added) return;

		void invalidate('app:session');
		await goto(resolve('/(app)/projects/[id=projectId]/setup', { id }));
	}
</script>

<PageTitle title="Add project" />
<Breadcrumbs
	items={[
		{ label: 'Projects', href: resolve('/(app)/projects') },
		{ label: 'Add project', href: resolve('/(app)/projects/new') }
	]}
/>

<form class="mx-auto mt-[6vh] flex max-w-140 flex-col gap-6" onsubmit={submit}>
	<div class="flex flex-col gap-3.5 text-center">
		<h1 class="text-hero font-bold">Add project</h1>
		<p class="-mt-1.5 mb-1.5 text-muted">One project for each app or service that sends errors.</p>
		<ProjectNameField bind:value={name} error={form.fields.name} variant="hero" />
		<FormActions {form} size="large" stretch>Add project</FormActions>
	</div>

	<Details title="Advanced settings" variant="disclosure" invalid={advancedInvalid}>
		{#snippet summary()}
			Color, logs to record, disk share. Defaults work for most projects.
		{/snippet}
		<FormGroup title="Color" hint={help.fields.Projects.Color}>
			<ColorField
				bind:value={values.color}
				name={name.trim()}
				error={form.fields.color}
				hideLegend
			/>
		</FormGroup>
		<FormGroup title="Logs to record">
			<LogLevelsField bind:value={values.logLevels} error={form.fields.log_levels} />
		</FormGroup>
		<FormGroup title="Disk share">
			<DiskShareField
				bind:value={values.diskShare}
				storage={data.storage}
				error={form.fields.disk_share}
				locked={data.defaults.disk_share_locked}
			/>
		</FormGroup>
	</Details>
</form>
