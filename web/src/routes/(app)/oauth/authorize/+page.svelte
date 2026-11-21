<script lang="ts">
	import { ApiError, send } from '#lib/api/client.ts';
	import Button from '#lib/components/Button.svelte';
	import Card from '#lib/components/Card.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let deciding: 'allow' | 'deny' | undefined = $state();
	let error = $state('');

	// The server checks the request again and answers where to go: the agent, with a code or an error.
	async function decide(allow: boolean) {
		if (deciding || !data.request) return;

		deciding = allow ? 'allow' : 'deny';
		error = '';

		try {
			const target = await send('authorization_decision', {
				query: data.query,
				form_token: data.request.form_token,
				allow
			});

			location.assign(target.redirect_url);
		} catch (failure) {
			error = failure instanceof ApiError ? failure.message : 'Could not send the answer.';
			deciding = undefined;
		}
	}
</script>

<PageHeading title="Connect a coding agent" />

<Card>
	{#if data.request}
		{@const request = data.request}
		<p>
			<b>{request.client_name}</b>{#if request.client_site}
				(published by <code>{request.client_site}</code>){/if} is asking to access {data.session
				.app_name}.
		</p>
		<p>
			This agent will act as you: list projects and issues, read error details and logs, and change
			issue statuses.{#if data.session.can_administer}
				As you are an admin, it can also manage projects, users and notification connections, asking
				you before it deletes anything or changes users, project settings or connections.{/if}
		</p>
		<p class="text-muted">
			Only allow an agent you started connecting. You will return to <code>
				{request.redirect_uri}
			</code>.
		</p>
		{#if error}<Notice tone="error">{error}</Notice>{/if}
		<div class="flex flex-wrap gap-2">
			<Button
				variant="primary"
				busy={deciding === 'allow'}
				disabled={deciding === 'deny'}
				onclick={() => decide(true)}
			>
				Allow
			</Button>
			<Button
				busy={deciding === 'deny'}
				disabled={deciding === 'allow'}
				onclick={() => decide(false)}
			>
				Deny
			</Button>
		</div>
	{:else}
		<div role="alert"><Notice tone="error">{data.refused}</Notice></div>
	{/if}
</Card>
