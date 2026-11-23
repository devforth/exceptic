<!--
@component
How to connect each MCP client, and the viewer's personal token. A new token lives only in this
component: gone after a reload or leaving the page, never in the address or storage.
-->
<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { PlainRequests } from '#lib/api/catalog.gen.ts';
	import { type Out, errorMessage, send } from '#lib/api/client.ts';
	import Button from '#lib/components/Button.svelte';
	import CodeBlock from '#lib/components/CodeBlock.svelte';
	import ConfirmButton from '#lib/components/ConfirmButton.svelte';
	import CopyButton from '#lib/components/CopyButton.svelte';
	import Details from '#lib/components/Details.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import SecretReveal from '#lib/components/SecretReveal.svelte';
	import Tabs from '#lib/components/Tabs.svelte';
	import type { Tab } from '#lib/components/tabs.ts';
	import { toast } from '#lib/toast.svelte.ts';

	type Props = { setup: Out<'get_mcp_setup'>; appName: string };

	let { setup, appName }: Props = $props();

	const clients: Tab[] = [
		{ id: 'claude', label: 'Claude Code' },
		{ id: 'codex', label: 'Codex' },
		{ id: 'other', label: 'Other' }
	];

	let token: PlainRequests['personal_token']['out'] | undefined = $state();
	let generating = $state(false);

	async function generate() {
		token = await send('personal_token');
		// token_set; the token itself is in no read.
		void invalidateAll();
	}

	async function generateFirst() {
		if (generating) return;

		generating = true;

		try {
			await generate();
		} catch (failure) {
			toast.show(errorMessage(failure, 'Could not generate the token.'), 'error');
		} finally {
			generating = false;
		}
	}

	const copyCommand = {
		copyTitle: 'Copy the command',
		copied: 'Command copied to clipboard.',
		copyVariant: 'neutral'
	} as const;
</script>

<p class="mb-1">Connect an MCP client to {appName}.</p>

{#if setup.unencrypted}
	<div class="mb-1.5">
		<Notice tone="error" alert>
			This server is reached over plain HTTP, so sign-in codes and tokens cross the network
			unencrypted. Serve it over HTTPS and set <code>PUBLIC_URL</code> to the https address.
		</Notice>
	</div>
{/if}

<Tabs label="MCP client" tabs={clients} param="client" variant="underline">
	{#snippet panel(client: string)}
		<div class="flex flex-col gap-3.5 pt-3.5">
			{#if client === 'claude'}
				<p class="text-muted">
					Run this command in your project's folder. Then use <code>/mcp</code> in Claude Code to sign
					in and allow access.
				</p>
				<CodeBlock
					variant="command"
					code={setup.claude_command}
					label="Claude Code command"
					{...copyCommand}
				/>
				<p class="text-xs text-muted">
					Add <code>--scope user</code> to make this server available in every project.
				</p>
				<Details variant="inline" title="Share the setup with your team">
					<div class="flex flex-col gap-2 pt-3.5">
						<p class="text-xs text-muted">
							Save this as <code>.mcp.json</code> in your repository. Each developer authorizes access
							separately.
						</p>
						<CodeBlock
							variant="snippet"
							code={setup.project_config}
							label=".mcp.json"
							copyTitle="Copy .mcp.json"
							copied="Configuration copied to clipboard."
							copyVariant="neutral"
						/>
					</div>
				</Details>
			{:else if client === 'codex'}
				<p class="text-muted">
					Run this command. Open the authorization URL in your browser, sign in and allow access.
				</p>
				<CodeBlock
					variant="command"
					code={setup.codex_command}
					label="Codex command"
					{...copyCommand}
				/>
			{:else}
				<p class="text-muted">
					Add this URL to your MCP client. With OAuth, sign in and allow access in your browser.
					Otherwise, use a personal token in the header below.
				</p>
				<CodeBlock
					variant="command"
					code={setup.url}
					label="MCP endpoint"
					copyTitle="Copy the endpoint"
					copied="Endpoint copied to clipboard."
					copyVariant="neutral"
				/>
				<p class="mt-2.5 text-muted">
					A personal token grants access on your behalf. Keep it secret.
				</p>
				{#if token}
					<p class="text-xs text-muted">Header <code>Authorization</code>, value:</p>
					<SecretReveal
						value="Bearer {token.token}"
						label="Authorization header value"
						copyTitle="Copy the authorization header value"
						copied="Header value copied to clipboard."
						copyVariant="neutral"
						masked
					>
						Copy and save this token now. After you reload or leave this page, it will no longer be
						available to copy. You will need to regenerate it if you lose it. Any previous personal
						token has stopped working.
					</SecretReveal>
				{:else if setup.token_set}
					<p class="text-xs text-muted">
						A personal token is active. Its value cannot be shown again. Regenerate it if you need a
						new copy.
					</p>
				{/if}
				<div class="flex flex-wrap items-center gap-2">
					{#if setup.token_set}
						<ConfirmButton
							variant="default"
							confirm={{
								title: 'Regenerate the personal token',
								message: 'Clients using the current token will lose access.',
								confirmLabel: 'Regenerate'
							}}
							action={generate}
						>
							Regenerate token
						</ConfirmButton>
					{:else}
						<Button busy={generating} onclick={generateFirst}>Generate token</Button>
					{/if}
					{#if token}
						<CopyButton
							text={token.setup_prompt}
							label="Copy setup prompt"
							title="Copy a prompt that has a coding agent add this server with the token"
							copied="Setup prompt copied: paste it to your agent."
							variant="neutral"
						/>
					{/if}
				</div>
			{/if}
		</div>
	{/snippet}
</Tabs>

{#if setup.ips_restricted}
	<p class="text-xs text-muted">
		Connections are restricted to addresses in <code>ALLOWED_IPS</code>.
	</p>
{/if}
