<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { post } from '#lib/api/client.ts';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import Button from '#lib/components/Button.svelte';
	import ConfirmButton from '#lib/components/ConfirmButton.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import {
		deleteConfirmation,
		deleteProject,
		projectInput,
		type ProjectFormValues
	} from '#lib/projects.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import ProjectForm from '../ProjectForm.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const project = $derived(data.project);
	const id = $derived(project.id);
	const fromEnv = $derived(project.from_env ?? false);

	const initial = $derived({
		name: project.name,
		color: project.shown_color,
		logLevels: project.log_levels ?? []
	});

	// The badges take a new color from the session, which the reload brings.
	async function save(values: ProjectFormValues) {
		await post('update_project', { project: String(id), ...projectInput(values) });
		toast.show('Saved.');
		void invalidateAll();
	}

	async function remove() {
		await deleteProject(project.id);
		await goto(resolve('/(app)/projects'));
	}
</script>

<Breadcrumbs
	items={[
		{ label: 'Projects', href: resolve('/(app)/projects') },
		{ label: project.name, href: resolve('/(app)/projects/[id=projectId]', { id }) }
	]}
/>
<PageHeading title={project.name}>
	{#snippet actions()}
		<Button icon="code" href={resolve('/(app)/projects/[id=projectId]/setup', { id })}>
			Show installation code
		</Button>
		{#if !fromEnv}
			<ConfirmButton icon="delete" confirm={deleteConfirmation(project.name)} action={remove}>
				Delete project
			</ConfirmButton>
		{/if}
	{/snippet}
</PageHeading>

{#if page.state.projectAdded}
	<div class="mb-4">
		<Notice tone="success">
			Project added. Put its DSN into the Sentry SDK config of the service.
		</Notice>
	</div>
{/if}

{#key project.id}
	<ProjectForm {initial} {fromEnv} dsn={project.dsn} submitLabel="Save" {save} />
{/key}
