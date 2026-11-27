<!--
@component
How a user signs in: the Google account linked to them, their sessions, their connected coding
agents (another user's only, when there are any) and their sign-in log,
read from get_activity (the viewer's own without userId). changeable allows disconnecting a session and
unlinking Google (one's own, or a user the viewer may take over); reason says why not.
-->
<script lang="ts">
	import { get, post, type Out } from '#lib/api/client.ts';
	import { browserName } from '#lib/format.ts';
	import AgentsTable from './AgentsTable.svelte';
	import Button from './Button.svelte';
	import ConfirmButton from './ConfirmButton.svelte';
	import Notice from './Notice.svelte';
	import StatusText from './StatusText.svelte';
	import Table from './Table.svelte';
	import type { TableColumn } from './table.ts';
	import Time from './Time.svelte';

	type Props = { userId?: string; changeable: boolean; reason?: string };

	let { userId, changeable, reason }: Props = $props();

	type Activity = Out<'get_activity'>;
	type Session = NonNullable<Activity['sessions']>[number];
	type SignIn = NonNullable<Activity['sign_ins']>[number];

	type Agent = NonNullable<Out<'list_agents'>['agents']>[number];

	let activity: Activity | undefined = $state();
	// Another user's connected coding agents (one's own are on the MCP page).
	let agents: Agent[] = $state([]);
	let failure = $state('');

	async function load() {
		try {
			const [read, owned] = await Promise.all([
				get('get_activity', userId ? { user_id: userId } : {}),
				userId ? get('list_agents', { user_id: userId }).catch(() => undefined) : undefined
			]);

			activity = read;
			agents = (owned?.agents ?? []).filter((agent) => agent.connected);
			failure = '';
		} catch {
			failure = 'The sessions could not be read.';
		}
	}

	$effect(() => {
		void load();
	});

	const target = $derived(userId ? { user_id: userId } : {});

	const methods: Record<string, string> = { password: 'Password', google: 'Google' };
	const results: Record<string, { text: string; tone: 'success' | 'error' | 'warning' }> = {
		ok: { text: 'Signed in', tone: 'success' },
		wrong_password: { text: 'Wrong password', tone: 'error' },
		blocked: { text: 'Blocked by the login limit', tone: 'warning' },
		not_linked: { text: 'Another Google account', tone: 'error' },
		unlinked: { text: 'Google unlinked', tone: 'error' }
	};

	type SessionColumn = TableColumn<'browser' | 'address' | 'signedIn' | 'active' | 'end'>;
	const sessionColumns: SessionColumn[] = [
		{ key: 'browser', label: 'Browser' },
		{ key: 'address', label: 'Address', wideOnly: true },
		{ key: 'signedIn', label: 'Signed in' },
		{ key: 'active', label: 'Last active' },
		{ key: 'end', label: 'Disconnect', actions: true }
	];

	type SignInColumn = TableColumn<'at' | 'method' | 'result' | 'address' | 'browser'>;
	const signInColumns: SignInColumn[] = [
		{ key: 'at', label: 'Time' },
		{ key: 'method', label: 'With' },
		{ key: 'result', label: 'Result' },
		{ key: 'address', label: 'Address', wideOnly: true },
		{ key: 'browser', label: 'Browser', wideOnly: true }
	];
</script>

{#if failure}
	<Notice tone="error">{failure}</Notice>
{:else if activity}
	<div class="flex flex-col gap-5">
		{#if activity.google_linked_at}
			<section class="flex flex-wrap items-center gap-2">
				<h3 class="font-semibold">Google</h3>
				<span class="text-muted">linked <Time at={activity.google_linked_at} /></span>
				<ConfirmButton
					variant="default"
					unavailable={changeable ? undefined : reason}
					confirm={{
						title: 'Unlink Google',
						message:
							'Unlink the Google account? Its sessions end, and Google links again only after a sign-in with the password.',
						confirmLabel: 'Unlink'
					}}
					action={async () => {
						await post('unlink_google', target);
						await load();
					}}
				>
					Unlink
				</ConfirmButton>
			</section>
		{/if}

		<section class="flex flex-col gap-2">
			<h3 class="font-semibold">Sessions</h3>
			<Table columns={sessionColumns} rows={activity.sessions ?? []} label="Sessions" compact flat>
				{#snippet cell(session: Session, column: SessionColumn)}
					{#if column.key === 'browser'}
						<span title={session.browser}>{browserName(session.browser) || '—'}</span>
					{:else if column.key === 'address'}
						{session.address || '—'}
					{:else if column.key === 'signedIn'}
						{methods[session.method ?? ''] ?? '—'} · <Time at={session.created_at} />
					{:else if column.key === 'active'}
						<Time at={session.last_active_at} />
					{:else}
						<!-- Both buttons as wide, whichever the row has. -->
						<span class="inline-grid w-28 [&>button]:w-full [&>button]:justify-center">
							{#if session.current}
								<Button variant="neutral" unavailable="The session you use: log out instead.">
									This session
								</Button>
							{:else}
								<ConfirmButton
									variant="danger"
									unavailable={changeable ? undefined : reason}
									confirm={{
										title: 'Disconnect session',
										message: 'Disconnect this session? That browser has to sign in again.',
										confirmLabel: 'Disconnect'
									}}
									action={async () => {
										await post('end_session', { ...target, session_id: session.id });
										await load();
									}}
								>
									Disconnect
								</ConfirmButton>
							{/if}
						</span>
					{/if}
				{/snippet}
				{#snippet empty()}No active session.{/snippet}
			</Table>
		</section>

		{#if agents.length}
			<section class="flex flex-col gap-2">
				<h3 class="font-semibold">Coding agents</h3>
				<AgentsTable
					{agents}
					compact
					unavailable={changeable ? undefined : reason}
					ondisconnect={() => void load()}
				/>
			</section>
		{/if}

		<section class="flex flex-col gap-2">
			<h3 class="font-semibold">Sign-in log</h3>
			<Table
				columns={signInColumns}
				rows={activity.sign_ins ?? []}
				label="Sign-in log"
				compact
				flat
			>
				{#snippet cell(signIn: SignIn, column: SignInColumn)}
					{#if column.key === 'at'}
						<Time at={signIn.at} />
					{:else if column.key === 'method'}
						{methods[signIn.method] ?? signIn.method}
					{:else if column.key === 'result'}
						{@const result = results[signIn.result] ?? { text: signIn.result, tone: 'warning' }}
						<StatusText tone={result.tone}>{result.text}</StatusText>
					{:else if column.key === 'address'}
						{signIn.address || '—'}
					{:else}
						<span title={signIn.browser}>{browserName(signIn.browser) || '—'}</span>
					{/if}
				{/snippet}
				{#snippet empty()}No sign-in yet.{/snippet}
			</Table>
		</section>
	</div>
{/if}
