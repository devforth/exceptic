<!--
@component
The form of a connection, new or saved: the type (new only) and its fields, then the name, the
projects and levels, and the options. The fields of an email connection open once its address
is confirmed. Saved tokens are never in the page: their fields stay empty to keep them.
-->
<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { help, levels, limits } from '#lib/api/catalog.gen.ts';
	import { post, type Out } from '#lib/api/client.ts';
	import Button from '#lib/components/Button.svelte';
	import Choice from '#lib/components/Choice.svelte';
	import ChoiceGroup from '#lib/components/ChoiceGroup.svelte';
	import Details from '#lib/components/Details.svelte';
	import LevelChoices from '#lib/components/LevelChoices.svelte';
	import LevelSummary from '#lib/components/LevelSummary.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import TextField from '#lib/components/TextField.svelte';
	import Time from '#lib/components/Time.svelte';
	import { FormState } from '#lib/form.svelte.ts';
	import {
		canTest,
		issueLinkFor,
		namePlaceholder,
		telegramUpdatesURL,
		typeLabel,
		type EmailStepState
	} from '#lib/notifications.ts';
	import { toggled } from '#lib/selection.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import ChatTestButton from './ChatTestButton.svelte';
	import DuplicateNotice from './DuplicateNotice.svelte';
	import EmailStep from './EmailStep.svelte';

	type Props = {
		defaults: Out<'get_connection_defaults'>;
		projects: { id: number; name: string }[];
		/** A saved connection; none for a new one. */
		connection?: Out<'get_connection'>;
	};

	let { defaults, projects, connection }: Props = $props();

	// The values the form starts with: the saved connection's, or a new one's defaults.
	function initial() {
		const types = defaults.types ?? [];
		const type = connection?.type ?? defaults.default_type;

		return {
			type,
			name: connection?.name ?? '',
			chatId: connection?.chat_id ?? '',
			slackChannel: connection?.slack_channel ?? '',
			allProjects: connection?.all_projects ?? true,
			projectIds: (connection?.project_ids ?? []).map(String),
			levels: connection?.levels ?? defaults.levels ?? [],
			active: connection?.active ?? true,
			includeIssueLink:
				connection?.include_issue_link ??
				types.find((choice) => choice.type === type)?.include_issue_link ??
				false
		};
	}

	const start = initial();
	const isNew = $derived(!connection);

	let type = $state(start.type);
	let name = $state(start.name);
	let botToken = $state('');
	let chatId = $state(start.chatId);
	let webhookURL = $state('');
	let slackBotToken = $state('');
	let slackChannel = $state(start.slackChannel);
	let allProjects = $state(start.allProjects);
	let projectIds: string[] = $state(start.projectIds);
	let chosenLevels: string[] = $state(start.levels);
	let active = $state(start.active);
	let includeIssueLink = $state(start.includeIssueLink);
	let issueLinkTouched = $state(false);
	// The email step of a new connection.
	let typedAddress = $state('');
	let emailProgress: EmailStepState = $state({ step: 'input', address: '', notice: '' });
	const emailVerified = $derived(emailProgress.step === 'verified');

	// After a save: the connection as stored (a save may clear fields), the typed tokens gone.
	function reload() {
		const stored = initial();

		({ name, chatId, slackChannel, allProjects, projectIds, active, includeIssueLink } = stored);
		chosenLevels = stored.levels;
		botToken = webhookURL = slackBotToken = '';
	}

	const form = new FormState();

	const typeChoices = $derived(defaults.types ?? []);
	const chosenType = $derived(typeChoices.find((choice) => choice.type === type));
	// A new email connection is filled in once its address is confirmed.
	const locked = $derived(isNew && type === 'email' && !emailVerified);
	const projectIdsAll = $derived(projects.map((project) => String(project.id)));
	const chosenProjects = $derived(
		projects.filter((project) => projectIds.includes(String(project.id)))
	);
	const updatesURL = $derived(telegramUpdatesURL(botToken));

	// What each chat field holds: typed, or a saved token (masked) that an empty field keeps.
	const saved: Record<string, string | undefined> = $derived({
		bot_token: connection?.saved_telegram_token,
		webhook_url: connection?.saved_webhook_url,
		slack_bot_token: connection?.saved_slack_bot_token
	});
	const chatValues = $derived({
		bot_token: botToken,
		chat_id: chatId,
		webhook_url: webhookURL,
		slack_bot_token: slackBotToken,
		slack_channel: slackChannel
	});
	const chatFieldsOf: Record<string, (keyof typeof chatValues)[]> = {
		telegram: ['bot_token', 'chat_id'],
		slack: ['webhook_url', 'slack_bot_token', 'slack_channel']
	};
	const testNeeds: Record<string, string> = {
		telegram: 'bot_token chat_id',
		slack: 'webhook_url|slack_bot_token slack_channel'
	};

	const filled = (field: string) =>
		(chatValues[field as keyof typeof chatValues] ?? '').trim() !== '' || Boolean(saved[field]);

	/** The chat fields of the type as typed: an empty one keeps the saved token. */
	function chatInput(): Partial<Record<keyof typeof chatValues, string>> {
		const input: Partial<Record<keyof typeof chatValues, string>> = {};

		for (const field of chatFieldsOf[type] ?? []) {
			const value = chatValues[field].trim();

			if (value) input[field] = value;
		}

		return input;
	}

	function chooseType(next: string) {
		type = next;
		const typeDefault = typeChoices.find((choice) => choice.type === next)?.include_issue_link;

		includeIssueLink = issueLinkFor(typeDefault ?? false, issueLinkTouched, includeIssueLink);
	}

	function testChat() {
		return post('test_chat', {
			...(connection ? { connection_id: connection.id } : { type }),
			name: name.trim(),
			include_issue_link: includeIssueLink,
			...chatInput()
		});
	}

	async function save(event: SubmitEvent) {
		event.preventDefault();

		const fields = {
			name: name.trim(),
			all_projects: allProjects,
			projects: allProjects ? [] : projectIds,
			levels: levels.event.filter((level) => chosenLevels.includes(level)),
			active,
			// Hidden without PUBLIC_URL: the stored value stays.
			...(defaults.issue_links_available ? { include_issue_link: includeIssueLink } : {}),
			...chatInput()
		};

		if (connection) {
			if (
				await form.submit(() =>
					post('update_connection', { connection_id: connection.id, ...fields })
				)
			) {
				toast.show('Saved.');
				await invalidateAll();
				reload();
			}

			return;
		}

		const email = type === 'email' ? { email: emailProgress.address } : {};

		if (await form.submit(() => post('create_connection', { type, ...email, ...fields }))) {
			toast.show('Connection added.');
			await goto(resolve('/(app)/notifications'));
		}
	}
</script>

<div class="flex flex-col gap-3">
	{#if isNew}
		<ChoiceGroup legend="Type" hint={help.fields.Notifications.Type} error={form.fields.type} card>
			{#each typeChoices as choice (choice.type)}
				<Choice
					type="radio"
					name="type"
					value={choice.type}
					label={typeLabel(choice.type)}
					checked={type === choice.type}
					disabled={!choice.available}
					reason={choice.reason}
					onchange={() => chooseType(choice.type)}
				/>
			{/each}
		</ChoiceGroup>
	{:else}
		<p class="text-muted">Type: <b class="text-text">{typeLabel(type)}</b></p>
		{#if form.fields.type}<Notice tone="error">{form.fields.type}</Notice>{/if}
	{/if}

	{#if type === 'email'}
		{#if connection}
			<p class="flex flex-wrap items-center gap-2">
				<b>{connection.email}</b>
				{#if connection.confirmed}
					<span class="text-sm text-success">
						✓ confirmed{#if connection.verified_at}&nbsp;<Time at={connection.verified_at} />{/if}
					</span>
				{/if}
			</p>
		{:else}
			<EmailStep
				bind:typed={typedAddress}
				bind:progress={emailProgress}
				error={form.fields.email}
			/>
		{/if}
	{/if}

	{#if connection && !connection.secret_readable}
		<Notice tone="warning">
			The saved token cannot be decrypted (SECRET_KEY changed?): enter it again.
		</Notice>
	{/if}

	<form id="connection-form" class="flex flex-col gap-3" onsubmit={save}>
		{#if type === 'telegram'}
			<TextField
				label="Bot token"
				name="bot_token"
				type="password"
				autocomplete="new-password"
				bind:value={botToken}
				required
				pattern={limits.telegramToken}
				placeholder="123456:ABC-DEF…"
				saved={saved.bot_token}
				hint={help.fields.Notifications['Telegram bot token']}
				error={form.fields.bot_token}
			>
				{#snippet action()}
					<Button
						variant="info"
						icon="externalLink"
						href={updatesURL}
						disabled={!updatesURL}
						external
						title="Opens the bot's getUpdates: write to the chat first, the chat ID is in the answer"
					>
						Find chat ID
					</Button>
				{/snippet}
			</TextField>
			<TextField
				label="Chat ID"
				name="chat_id"
				bind:value={chatId}
				required
				pattern={limits.telegramChat}
				placeholder="-1001234567890 or @channel"
				hint={help.fields.Notifications['Chat ID']}
				error={form.fields.chat_id}
			/>
			<ChatTestButton ready={canTest(testNeeds.telegram, filled)} test={testChat} />
			<p class="text-xs text-muted">
				Create a bot with
				<a href="https://t.me/BotFather" target="_blank" rel="noopener noreferrer">@BotFather</a>
				(send <code>/newbot</code>, it replies with the token) and add the bot to the chat. Then
				write a message in the chat and press <b>Find chat ID</b>: the ID is
				<code>chat.id</code> in the answer (<code>getUpdates</code>).
				<a
					href="https://core.telegram.org/bots/tutorial#obtain-your-bot-token"
					target="_blank"
					rel="noopener noreferrer"
				>
					Telegram's guide
				</a>. Tokens are stored encrypted.
			</p>
		{:else if type === 'slack'}
			<TextField
				label="Incoming webhook URL"
				name="webhook_url"
				type="password"
				autocomplete="new-password"
				bind:value={webhookURL}
				placeholder="https://hooks.slack.com/services/…"
				saved={saved.webhook_url}
				hint={help.fields.Notifications['Incoming webhook URL']}
				error={form.fields.webhook_url}
			/>
			<p class="text-xs text-muted">
				<a href="https://api.slack.com/apps?new_app=1" target="_blank" rel="noopener noreferrer">
					Create a Slack app
				</a>, turn on Incoming Webhooks and add one for the channel.
				<a
					href="https://api.slack.com/messaging/webhooks"
					target="_blank"
					rel="noopener noreferrer"
				>
					Slack's guide
				</a>. Mattermost and Rocket.Chat webhooks work too.
			</p>
			<TextField
				label="Bot token"
				note="optional"
				name="slack_bot_token"
				type="password"
				autocomplete="new-password"
				bind:value={slackBotToken}
				pattern={limits.slackToken}
				placeholder="xoxb-…"
				saved={saved.slack_bot_token}
				hint={help.fields.Notifications['Slack bot token']}
				error={form.fields.slack_bot_token}
			/>
			<TextField
				label="Channel ID"
				name="slack_channel"
				bind:value={slackChannel}
				pattern={limits.slackChannel}
				placeholder="C0123456789"
				hint={help.fields.Notifications['Channel ID']}
				error={form.fields.slack_channel}
			/>
			<ChatTestButton ready={canTest(testNeeds.slack, filled)} test={testChat} />
			<p class="text-xs text-muted">
				Bot:
				<a href="https://api.slack.com/apps" target="_blank" rel="noopener noreferrer">
					your Slack app
				</a>
				→ OAuth &amp; Permissions → Bot Token Scopes: <code>chat:write</code> → Install to Workspace
				→ Bot User OAuth Token. Invite the bot to the channel (<code>/invite @app</code>); the
				channel ID is at the bottom of the channel details. Tokens and webhooks are stored
				encrypted.
			</p>
		{/if}

		{#if form.duplicate}<DuplicateNotice duplicate={form.duplicate} />{/if}

		<fieldset class="flex min-w-0 flex-col gap-3 disabled:*:not-last:opacity-50" disabled={locked}>
			<TextField
				label="Name"
				name="name"
				bind:value={name}
				maxlength={limits.connectionName}
				placeholder={namePlaceholder(type, typedAddress, chosenType?.name_placeholder ?? '')}
				description="Optional: left empty, it is filled in when saved."
				error={form.fields.name}
			/>

			<Details
				title="Projects"
				variant="form"
				hint={help.fields.Notifications.Projects}
				invalid={Boolean(form.fields.projects)}
			>
				{#snippet summary()}
					{#if allProjects}
						All projects
					{:else if chosenProjects.length > 0}
						{chosenProjects.map((project) => project.name).join(', ')}
					{:else}
						<span class="text-muted">none</span>
					{/if}
				{/snippet}
				<ChoiceGroup legend="Projects that are sent" hideLegend inline error={form.fields.projects}>
					{#snippet lead()}
						<Choice
							type="checkbox"
							label="All projects, including ones added later"
							checked={allProjects}
							onchange={(checked: boolean) => (allProjects = checked)}
						/>
					{/snippet}
					{#if !allProjects}
						{#each projects as project (project.id)}
							<Choice
								type="checkbox"
								value={String(project.id)}
								label={project.name}
								checked={projectIds.includes(String(project.id))}
								onchange={(checked: boolean) =>
									(projectIds = toggled(projectIdsAll, projectIds, String(project.id), checked))}
							/>
						{/each}
					{/if}
				</ChoiceGroup>
			</Details>

			<Details
				title="Levels"
				variant="form"
				hint={help.fields.Notifications.Levels}
				invalid={Boolean(form.fields.levels)}
			>
				{#snippet summary()}<LevelSummary kind="event" chosen={chosenLevels} />{/snippet}
				<LevelChoices
					kind="event"
					legend="Levels that are sent"
					hideLegend
					inline={false}
					selected={chosenLevels}
					onchange={(chosen: string[]) => (chosenLevels = chosen)}
					error={form.fields.levels}
				/>
			</Details>

			<ChoiceGroup legend="Options" hideLegend inline error={form.fields.active}>
				<Choice
					type="checkbox"
					label="Active: send notifications"
					hint={help.fields.Notifications.Active}
					checked={active}
					onchange={(checked: boolean) => (active = checked)}
				/>
				{#if defaults.issue_links_available}
					<Choice
						type="checkbox"
						label="Include Open issue link"
						hint={help.fields.Notifications['Include Open issue link']}
						checked={includeIssueLink}
						onchange={(checked: boolean) => {
							includeIssueLink = checked;
							issueLinkTouched = true;
						}}
					/>
				{/if}
			</ChoiceGroup>

			{#if form.error}<Notice tone="error">{form.error}</Notice>{/if}

			<div class="flex gap-2">
				<Button type="submit" variant="primary" busy={form.busy}>{isNew ? 'Add' : 'Save'}</Button>
				<Button href={resolve('/(app)/notifications')}>Cancel</Button>
			</div>
		</fieldset>
	</form>
</div>
