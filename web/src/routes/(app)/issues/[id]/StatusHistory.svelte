<!--
@component
Who changed an issue's status and when, newest first; a resolved issue that happened again, or
an ignored one that reached its count of events, is reopened by its event, which the entry links to.
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import Button from '#lib/components/Button.svelte';
	import StatusBadge from '#lib/components/StatusBadge.svelte';
	import Time from '#lib/components/Time.svelte';
	import { withQuery } from '#lib/nav.ts';
	import { changeAuthor, type StatusChange } from './issue.ts';

	let { issueId, history }: { issueId: string; history: StatusChange[] } = $props();
</script>

<ol>
	{#each history as change, index (index)}
		<li class="flex flex-wrap items-baseline gap-1.5 border-b border-border py-1.5 last:border-b-0">
			<StatusBadge status={change.from} /> → <StatusBadge status={change.to} />
			<span>{changeAuthor(change)}</span>
			{#if change.by_event && change.event_id}
				<Button
					variant="link"
					href={withQuery(resolve('/(app)/issues/[id]', { id: issueId }), {
						event: change.event_id
					})}
				>
					(event)
				</Button>
			{/if}
			<span class="text-xs text-muted"><Time at={change.created_at} /></span>
		</li>
	{/each}
</ol>
