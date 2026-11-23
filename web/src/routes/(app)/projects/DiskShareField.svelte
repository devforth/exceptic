<!--
@component
The disk share of a project: the database's budget, the share on a slider with what it allows,
what a saved project uses and the history it keeps now (from get_storage) and how the cleanup by size uses it, told from
the generated steps and rules of the cleanup.
-->
<script lang="ts">
	import { cleanupRules, cleanupSteps, diskShare, help } from '#lib/api/catalog.gen.ts';
	import type { Out } from '#lib/api/client.ts';
	import Facts from '#lib/components/Facts.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import Slider from '#lib/components/Slider.svelte';
	import { formatSize } from '#lib/format.ts';
	import { keptFacts } from '#lib/projects.ts';

	type Props = {
		value: number;
		storage: Out<'get_storage'>;
		/** A saved project: what it keeps now. */
		projectId?: number;
		error?: string;
	};

	let { value: share = $bindable(), storage, projectId, error }: Props = $props();

	const budget = $derived(storage.limit_bytes ?? 0);
	const usage = $derived(storage.projects?.find((part) => part.id === projectId));
	const uses = $derived([
		{
			label: 'Uses now',
			value: usage?.size ?? '0 B',
			detail: budget ? `of the ${storage.limit} budget` : undefined
		}
	]);
	const kept = $derived(keptFacts(usage?.oldest));

	type Fact = { label: string; value: string; detail?: string };

	const always = cleanupSteps.filter((step) => step.always).map((step) => step.name);
	const rest = cleanupSteps.filter((step) => !step.always).map((step) => step.name);
</script>

{#snippet value(fact: Fact)}
	<b>{fact.value}</b>
	{#if fact.detail}<span class="text-muted">{fact.detail}</span>{/if}
{/snippet}

<div class="flex flex-col gap-2.5">
	{#if budget}
		<p>
			Disk budget for all projects: <b>{storage.limit}</b>
			<span class="text-muted">· {storage.used} used</span>
		</p>
	{:else}
		<p class="text-muted">
			MAX_DATABASE_SIZE is off: nothing is deleted, so the share applies only once it is set.
		</p>
	{/if}

	<Slider
		label="Allow this project to use at most"
		name="disk_share"
		bind:value={share}
		max={diskShare.max}
		step={diskShare.step}
		least={diskShare.min}
		marks={[0, 50, 100]}
		unit="%"
		hint={help.fields.Projects['Disk share']}
		{error}
	>
		{#snippet output()}
			<b>{share}%</b>
			{#if budget}<span class="text-muted">· up to {formatSize((budget * share) / 100)}</span>{/if}
		{/snippet}
	</Slider>

	{#if projectId !== undefined}
		<Facts facts={uses} boxed {value} />
		<Facts
			facts={kept}
			boxed
			title="History kept"
			hint={help.fields.Projects['History kept']}
			{value}
		/>
	{/if}

	<Notice tone="info">
		<div class="flex flex-col gap-1.5 text-label">
			<p>
				<b>How it works.</b> Errors and logs are kept until the database reaches
				{cleanupRules.startPercent}% of its limit; nothing is deleted before that. Then the cleanup
				deletes {always.join(', ')}; while the database is still above {cleanupRules.stopPercent}%,
				it goes on, one step after another, with {rest.join(', ')}.
			</p>
			<p>
				A month there is {cleanupRules.ageDays} days and a week {cleanupRules.recentAgeDays}; an
				issue keeps its newest {cleanupRules.oldEventsPerIssue} events until the steps that leave only
				its latest.
			</p>
			<p>
				The share matters only then: the project keeps no more than this share of the budget, its
				oldest records going first, so one flooded project cannot push out the older, still
				important records of the others.
			</p>
			<p class="text-muted">{diskShare.max}% (the default) means no limit of its own.</p>
		</div>
	</Notice>
</div>
