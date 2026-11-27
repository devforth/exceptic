<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { post } from '#lib/api/client.ts';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import ConfirmButton from '#lib/components/ConfirmButton.svelte';
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

	const initial = $derived({
		color: project.shown_color,
		logLevels: project.log_levels ?? [],
		diskShare: project.disk_share,
		allowedOrigins: (project.allowed_origins ?? []).join(', ')
	});

	// The badges take a new name or color from the session, which the reload brings.
	async function save(values: ProjectFormValues, name: string) {
		await post('update_project', { project: String(id), name, ...projectInput(values) });
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
{#key project.id}
	<ProjectForm
		{id}
		name={project.name}
		{initial}
		storage={data.storage}
		{save}
		shareEditable={data.session.can_administer}
	>
		{#snippet actions()}
			{#if data.session.can_administer}
				<ConfirmButton icon="delete" confirm={deleteConfirmation(project.name)} action={remove}>
					Delete project
				</ConfirmButton>
			{/if}
		{/snippet}
	</ProjectForm>
{/key}
