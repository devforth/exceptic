<script lang="ts">
	import { goto, invalidate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { post } from '#lib/api/client.ts';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import { projectInput, type ProjectFormValues } from '#lib/projects.ts';
	import ProjectForm from '../ProjectForm.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const initial = $derived({
		name: '',
		color: data.defaults.shown_color,
		logLevels: data.defaults.log_levels ?? []
	});

	// The new project's page says what to do with its DSN.
	async function add(values: ProjectFormValues) {
		const input = projectInput(values);
		const project = await post('create_project', { ...input, name: input.name ?? '' });

		void invalidate('app:session');
		await goto(resolve('/(app)/projects/[id=projectId]', { id: project.id }), {
			state: { projectAdded: true }
		});
	}
</script>

<Breadcrumbs
	items={[
		{ label: 'Projects', href: resolve('/(app)/projects') },
		{ label: 'Add project', href: resolve('/(app)/projects/new') }
	]}
/>
<PageHeading title="Add project" />
<p class="mb-4 text-muted">One project for each app or service that sends errors.</p>

<ProjectForm {initial} submitLabel="Add" save={add} />
