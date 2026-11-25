<!--
@component
Who changed an issue's status and when, newest first, down to its creation (always unresolved,
at its first seen); a resolved issue that happened again, or an ignored one that reached its
count of events, is reopened by its event, which the entry links to.
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import Button from '#lib/components/Button.svelte';
	import StatusBadge from '#lib/components/StatusBadge.svelte';
	import Time from '#lib/components/Time.svelte';
	import { withQuery } from '#lib/nav.ts';
	import { changeAuthor, type StatusChange } from './issue.ts';

	type Props = { issueId: string; history: StatusChange[]; createdAt: string };

	let { issueId, history, createdAt }: Props = $props();
</script>

<!-- A grid: the statuses in one column as wide as the widest, so who and when line up. -->
<ol class="grid grid-cols-[max-content_1fr] gap-x-3">
	{#each history as change, index (index)}
		<li class="col-span-2 grid grid-cols-subgrid items-baseline border-b border-border py-1.5">
			<StatusBadge status={change.to} />
			<span class="flex flex-wrap items-baseline gap-2">
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
			</span>
		</li>
	{/each}
	<li class="col-span-2 grid grid-cols-subgrid items-baseline py-1.5">
		<StatusBadge status="unresolved" />
		<span class="flex flex-wrap items-baseline gap-2">
			<span>created</span>
			<span class="text-xs text-muted"><Time at={createdAt} /></span>
		</span>
	</li>
</ol>
