<!--
@component
A choice of projects in a form card: All projects (including ones added later) or some, one per
line as badges; the heading shows what is chosen. `allowAll` false disables All with its reason
(a viewer without every project cannot give it); `locked` disables the whole field with why.
`required`: at least one project, so the last one checked cannot be unchecked, and turning All off
checks every project offered.
-->
<script lang="ts">
	import Badge from './Badge.svelte';
	import Choice from './Choice.svelte';
	import ChoiceGroup from './ChoiceGroup.svelte';
	import Details from './Details.svelte';
	import ProjectsSummary from './ProjectsSummary.svelte';
	import { toggled } from '#lib/selection.ts';

	type Props = {
		/** The projects that can be chosen. */
		projects: { id: number; name: string }[];
		allProjects: boolean;
		/** The chosen ids as strings, in the list's order. */
		projectIds: string[];
		/** The group's name for screen readers; the title names it on screen. */
		legend: string;
		title?: string;
		hint?: string;
		error?: string;
		allowAll?: boolean;
		/** Why the field cannot change; it is then disabled. */
		locked?: string;
		/** At least one project stays chosen. */
		required?: boolean;
	};

	let {
		projects,
		allProjects = $bindable(),
		projectIds = $bindable(),
		legend,
		title = 'Projects',
		hint,
		error,
		allowAll = true,
		locked,
		required = false
	}: Props = $props();

	const known = $derived(projects.map((project) => String(project.id)));
	const lastReason = 'At least one project.';
	const chosen = $derived(projects.filter((project) => projectIds.includes(String(project.id))));
	// Without allowAll, All can still be turned off, never on.
	const allReason = $derived(
		locked ??
			(allowAll || allProjects
				? undefined
				: 'Only someone with access to all projects can give it.')
	);
</script>

<Details {title} variant="form" {hint} invalid={Boolean(error)}>
	{#snippet summary()}
		<ProjectsSummary all={allProjects} names={chosen.map((project) => project.name)} />
	{/snippet}
	<ChoiceGroup {legend} hideLegend compact {error}>
		{#snippet lead()}
			<Choice
				type="checkbox"
				label="All projects, including ones added later"
				checked={allProjects}
				disabled={Boolean(allReason)}
				reason={allReason}
				onchange={(checked: boolean) => {
					allProjects = checked;

					// Off with nothing chosen: every project, so one stays.
					if (!checked && required && !chosen.length) {
						projectIds = known;
					}
				}}
			/>
		{/snippet}
		{#if !allProjects}
			{#each projects as project (project.id)}
				{@const last =
					required && chosen.length === 1 && chosen[0].id === project.id ? lastReason : undefined}
				<Choice
					type="checkbox"
					value={String(project.id)}
					label={project.name}
					checked={projectIds.includes(String(project.id))}
					disabled={Boolean(locked ?? last)}
					reason={locked ?? last}
					onchange={(checked: boolean) =>
						(projectIds = toggled(known, projectIds, String(project.id), checked))}
				>
					<Badge tone="project" label={project.name} />
				</Choice>
			{/each}
		{/if}
	</ChoiceGroup>
</Details>
