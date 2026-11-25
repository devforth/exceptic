<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { logSearchSeconds } from '#lib/api/catalog.gen.ts';
	import { get, type Item, errorMessage } from '#lib/api/client.ts';
	import AgentCopyButton from '#lib/components/AgentCopyButton.svelte';
	import Badge from '#lib/components/Badge.svelte';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import Button from '#lib/components/Button.svelte';
	import Card from '#lib/components/Card.svelte';
	import CodeBlock from '#lib/components/CodeBlock.svelte';
	import CopyButton from '#lib/components/CopyButton.svelte';
	import Details from '#lib/components/Details.svelte';
	import Facts from '#lib/components/Facts.svelte';
	import IssueActions from '#lib/components/IssueActions.svelte';
	import { doneMessage, type IssueAction } from '#lib/components/issueActions.ts';
	import LevelBadge from '#lib/components/LevelBadge.svelte';
	import LogBody from '#lib/components/LogBody.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import StatusBadge from '#lib/components/StatusBadge.svelte';
	import { ignoreBoundText } from '#lib/components/issueStatus.ts';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import { clockTime } from '#lib/format.ts';
	import { withQuery } from '#lib/nav.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import type { PageProps } from './$types';
	import ExceptionCard from './ExceptionCard.svelte';
	import {
		changeAuthor,
		eventNotice,
		eventsShownText,
		notStoredText,
		type EventDetails,
		type IssueDetails
	} from './issue.ts';
	import StatusHistory from './StatusHistory.svelte';

	let { data }: PageProps = $props();

	const details = $derived(data.details);
	const issue = $derived(details.issue);
	const event = $derived(details.event ?? undefined);
	const history = $derived(details.history ?? []);
	const canAdminister = $derived(page.data.session?.can_administer ?? false);

	const issuesHref = resolve('/(app)/issues');
	const issueHref = $derived(resolve('/(app)/issues/[id]', { id: issue.id }));
	const issuesWith = (params: Record<string, string>) => withQuery(issuesHref, params);
	const eventHref = (eventId: string) => withQuery(issueHref, { event: eventId });

	async function acted(action: IssueAction) {
		toast.show(doneMessage(action, 1));

		if (action === 'delete') {
			await goto(issuesHref);
		} else {
			// The status, the buttons, the history and the menu's count.
			await invalidateAll();
		}
	}

	// The events: the newest come with the page, Load more adds the older ones.
	type EventRow = Item<'list_issue_events'>;
	type MoreEvents = { of: IssueDetails; rows: EventRow[]; cursor?: string };

	// Raw: compared with the page's details, which a proxy would never equal.
	let more: MoreEvents | undefined = $state.raw();
	let loadingMore = $state(false);
	const events = $derived(more?.of === details ? more.rows : (details.events.items ?? []));
	const eventsCursor = $derived(more?.of === details ? more.cursor : details.events.next_cursor);

	async function loadMore() {
		loadingMore = true;

		try {
			const next = await get('list_issue_events', { issue_id: issue.id, cursor: eventsCursor });

			more = { of: details, rows: [...events, ...(next.items ?? [])], cursor: next.next_cursor };
		} catch (failure) {
			toast.show(errorMessage(failure, 'Could not load more events.'), 'error');
		} finally {
			loadingMore = false;
		}
	}

	type Fact = { label: string; key: string };

	const issueFacts: Fact[] = $derived(
		[
			{ label: 'ID', key: 'id' },
			{ label: 'Project', key: 'project' },
			{ label: 'Status', key: 'status' },
			{ label: 'Events', key: 'events' },
			{ label: 'Not stored', key: 'notStored' },
			{ label: 'First seen', key: 'firstSeen' },
			{ label: 'Last seen', key: 'lastSeen' },
			{ label: 'Resolved', key: 'resolved' }
		].filter(
			(fact) =>
				(fact.key !== 'resolved' || issue.resolved_at) &&
				(fact.key !== 'notStored' || issue.not_stored)
		)
	);

	function eventFacts(shown: EventDetails): Fact[] {
		const optional: [string, string, unknown][] = [
			['Environment', 'environment', shown.environment],
			['Release', 'release', shown.release],
			['Server', 'server', shown.server_name],
			['Platform', 'platform', shown.platform],
			['SDK', 'sdk', shown.sdk],
			['Trace', 'trace', shown.trace_id]
		];

		return [
			{ label: 'Time', key: 'time' },
			...optional.filter(([, , value]) => value).map(([label, key]) => ({ label, key }))
		];
	}

	type EventColumn = TableColumn<'event' | 'received' | 'environment' | 'release'>;
	const eventColumns: EventColumn[] = [
		{ key: 'event', label: 'Event' },
		{ key: 'received', label: 'Received' },
		{ key: 'environment', label: 'Environment' },
		{ key: 'release', label: 'Release' }
	];

	type Log = NonNullable<EventDetails['related_logs']>[number];
	type LogColumn = TableColumn<'time' | 'level' | 'body'>;
	const logColumns: LogColumn[] = [
		{ key: 'time', label: 'Time' },
		{ key: 'level', label: 'Level' },
		{ key: 'body', label: 'Message' }
	];

	type Breadcrumb = NonNullable<EventDetails['breadcrumbs']>[number];
	type BreadcrumbColumn = TableColumn<'time' | 'category' | 'level' | 'message'>;
	const breadcrumbColumns: BreadcrumbColumn[] = [
		{ key: 'time', label: 'Time' },
		{ key: 'category', label: 'Category' },
		{ key: 'level', label: 'Level' },
		{ key: 'message', label: 'Message' }
	];

	// Headers, tags and the user: names and values.
	type Pair = { name: string; value: string };
	type PairColumn = TableColumn<'name' | 'value'>;
	const pairColumns: PairColumn[] = [
		{ key: 'name', label: 'Name', rowHeader: true },
		{ key: 'value', label: 'Value' }
	];

	function userPairs(user: EventDetails['user']): Pair[] {
		const fields: [string, string | undefined][] = [
			['ID', user?.id],
			['Email', user?.email],
			['Username', user?.username],
			['IP address', user?.ip_address]
		];

		return fields.flatMap(([name, value]) => (value ? [{ name, value }] : []));
	}
</script>

{#snippet pairs(rows: Pair[], label: string)}
	<Table columns={pairColumns} {rows} {label} compact scroll headerless>
		{#snippet cell(pair: Pair, column: PairColumn)}{pair[column.key]}{/snippet}
	</Table>
{/snippet}

<Breadcrumbs
	items={[
		{ label: 'Issues', href: issuesHref },
		{ label: issue.title, href: issueHref }
	]}
/>

{#snippet locationLine()}{issue.location}{/snippet}

<PageHeading title={issue.title} description={issue.location ? locationLine : undefined}>
	{#snippet leading()}
		<LevelBadge level={issue.level} href={issuesWith({ level: issue.level })} />
	{/snippet}
</PageHeading>

<Facts facts={issueFacts} columns>
	{#snippet value(fact: Fact)}
		{#if fact.key === 'id'}
			{issue.id}
		{:else if fact.key === 'project'}
			<Badge
				tone="project"
				label={issue.project}
				href={issuesWith({ project: issue.project })}
				title="Issues of {issue.project}"
			/>
		{:else if fact.key === 'status'}
			<StatusBadge
				status={issue.status}
				href={issuesWith({ status: issue.status })}
				detail={issue.status === 'ignored' ? ignoreBoundText(issue) : undefined}
			/>
			{#if history[0]}
				<span class="text-xs text-muted">
					{changeAuthor(history[0])} · <Time at={history[0].created_at} />
				</span>
			{/if}
		{:else if fact.key === 'events'}
			{#if events.length}
				<a href="#events" title="Show the events">{issue.event_count}</a>
			{:else}
				{issue.event_count}
			{/if}
		{:else if fact.key === 'notStored'}
			{notStoredText(issue.not_stored ?? 0)}
		{:else if fact.key === 'firstSeen'}
			<Time at={issue.first_seen} />
		{:else if fact.key === 'lastSeen'}
			<Time at={issue.last_seen} />
		{:else if issue.resolved_at}
			<Time at={issue.resolved_at} />
		{/if}
	{/snippet}
</Facts>

<div class="mb-6 flex flex-wrap items-center gap-2">
	<IssueActions
		issueIds={[issue.id]}
		status={issue.status}
		canDelete={canAdminister}
		ondone={acted}
	/>
	<span class="ml-auto flex flex-wrap gap-2">
		<CopyButton
			text={details.agent_prompt}
			label="Prompt"
			title="Copy prompt for a coding agent"
			copied="Agent prompt copied to clipboard."
		/>
		<AgentCopyButton request={issue.mcp_reference ?? ''} />
	</span>
</div>

<div class="flex flex-col gap-4">
	{#if event}
		<section>
			<h2 class="text-h2 font-bold">Event <code class="text-sm">{event.event_id}</code></h2>
			{#if event.thinned}
				<Notice tone="info">
					Details of this old event (breadcrumbs, request, user, tags, contexts, extra, local
					variables) were removed to save space; the error, its stack trace and message stay.
				</Notice>
			{/if}
			<Facts facts={eventFacts(event)} columns>
				{#snippet value(fact: Fact)}
					{#if fact.key === 'time'}
						<Time at={event.timestamp} />
						{#if event.clock_drift}
							<span class="text-xs text-muted">
								the device's clock was off by {event.clock_drift}
							</span>
						{/if}
					{:else if fact.key === 'environment'}
						{event.environment}
					{:else if fact.key === 'release'}
						{event.release}
					{:else if fact.key === 'server'}
						{event.server_name}
					{:else if fact.key === 'platform'}
						{event.platform}
					{:else if fact.key === 'sdk'}
						{event.sdk}
					{:else if event.trace_has_logs}
						<Button
							variant="link"
							href={withQuery(resolve('/(app)/logs'), { trace_id: event.trace_id ?? '' })}
							title="Logs of this trace"
						>
							<code>{event.trace_id}</code>
						</Button>
					{:else}
						<code>{event.trace_id}</code>
					{/if}
				{/snippet}
			</Facts>
		</section>

		{#if event.exceptions?.length}
			<Card title="Exception">
				{#each event.exceptions as exception, index (index)}
					<ExceptionCard {exception} cause={index > 0} />
				{/each}
			</Card>
		{:else if event.message}
			<Card title="Message">
				<CodeBlock code={event.message} label="Message" />
			</Card>
		{/if}
	{:else}
		<Notice tone="info">
			{eventNotice(details.event_state)}
			{#if details.event_state === 'missing'}
				<Button variant="link" href={issueHref}>Show the latest event</Button>
			{/if}
		</Notice>
	{/if}

	{#if history.length}
		<Card title="Status history">
			<StatusHistory issueId={issue.id} {history} />
		</Card>
	{/if}

	{#if events.length}
		<section id="events" class="scroll-mt-4">
			<Card title="Events">
				<Table
					columns={eventColumns}
					rows={events}
					label="Events"
					rowKey={(row: EventRow) => row.event_id}
					selectedKey={event?.event_id}
					compact
					scroll
				>
					{#snippet cell(row: EventRow, column: EventColumn)}
						{#if column.key === 'event'}
							<Button variant="link" href={eventHref(row.event_id)}>
								<code>{row.event_id.slice(0, 8)}</code>
							</Button>
						{:else if column.key === 'received'}
							<Time at={row.received_at} />
						{:else if column.key === 'environment'}
							{row.environment ?? ''}
						{:else}
							{row.release ?? ''}
						{/if}
					{/snippet}
				</Table>
				<div class="flex flex-wrap items-center gap-3 text-xs text-muted">
					{eventsShownText(events.length, issue.stored_events) ?? ''}
					{#if eventsCursor}
						<Button busy={loadingMore} onclick={loadMore}>Load more</Button>
					{/if}
				</div>
			</Card>
		</section>
	{/if}

	{#if event}
		{#if event.related_logs?.length || event.logs_stopped}
			<Details
				title={event.logs_from_request ? 'Logs from the same request' : 'Logs from the same trace'}
				count={event.related_logs?.length ?? 0}
			>
				{#if event.logs_stopped}
					<p class="text-xs text-muted">
						The search for these logs stopped after {logSearchSeconds} s, so there may be more: open the
						trace's logs to search further.
					</p>
				{/if}
				{#if !event.logs_from_request && event.related_logs?.length}
					<p class="text-xs text-muted">
						Not linked to this request exactly: logs of the same trace within a minute of the error.
					</p>
				{/if}
				<Table
					columns={logColumns}
					rows={event.related_logs ?? []}
					label="Related logs"
					compact
					scroll
					headerless
				>
					{#snippet cell(log: Log, column: LogColumn)}
						{#if column.key === 'time'}
							<Time at={log.time} />
						{:else if column.key === 'level'}
							<LevelBadge level={log.level} />
						{:else}
							<LogBody text={log.body} />
						{/if}
					{/snippet}
				</Table>
			</Details>
		{/if}

		{#if event.breadcrumbs?.length}
			<Details title="Breadcrumbs" count={event.breadcrumbs.length}>
				<Table
					columns={breadcrumbColumns}
					rows={event.breadcrumbs}
					label="Breadcrumbs"
					compact
					scroll
				>
					{#snippet cell(crumb: Breadcrumb, column: BreadcrumbColumn)}
						{#if column.key === 'time'}
							{#if crumb.timestamp}<span class="whitespace-nowrap">
									{clockTime(crumb.timestamp)}
								</span>{/if}
						{:else if column.key === 'category'}
							{crumb.category ?? ''}
						{:else if column.key === 'level'}
							{#if crumb.level}<LevelBadge level={crumb.level} />{/if}
						{:else}
							{#if crumb.message}<LogBody text={crumb.message} />{/if}
							{#if crumb.data}
								<Details variant="inline" title="data">
									<CodeBlock code={crumb.data} label="Breadcrumb data" />
								</Details>
							{/if}
						{/if}
					{/snippet}
				</Table>
			</Details>
		{/if}

		{#if event.request}
			<Details title="Request" count={event.request.headers?.length}>
				{#snippet summary()}<code>{event.request?.method} {event.request?.url}</code>{/snippet}
				{@render pairs(event.request.headers ?? [], 'Request headers')}
			</Details>
		{/if}

		{#if event.tags?.length || event.user}
			<div class="grid grid-cols-[repeat(auto-fit,minmax(320px,1fr))] items-start gap-4">
				{#if event.tags?.length}
					<Details title="Tags" count={event.tags.length}>
						{@render pairs(event.tags, 'Tags')}
					</Details>
				{/if}
				{#if event.user}
					<Details title="User">
						{@render pairs(userPairs(event.user), 'User')}
					</Details>
				{/if}
			</div>
		{/if}

		{#if event.contexts?.length}
			<Details title="Contexts" count={event.contexts.length}>
				{#each event.contexts as context (context.name)}
					<Details variant="inline" title={context.name}>
						<CodeBlock code={context.value} label="Context {context.name}" />
					</Details>
				{/each}
			</Details>
		{/if}

		<Details title="Raw event JSON">
			<CodeBlock code={event.payload} label="The event as stored" />
		</Details>
	{/if}
</div>
