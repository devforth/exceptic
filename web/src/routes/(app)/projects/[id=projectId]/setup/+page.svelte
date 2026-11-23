<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import BackButton from '#lib/components/BackButton.svelte';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import Button from '#lib/components/Button.svelte';
	import CodeBlock from '#lib/components/CodeBlock.svelte';
	import CopyButton from '#lib/components/CopyButton.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import Tabs from '#lib/components/Tabs.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const setup = $derived(data.setup);
	const platforms = $derived(setup.platforms ?? []);
	const id = $derived(setup.id);
	const canAdminister = $derived(data.session.can_administer);
	const tabs = $derived(platforms.map((platform) => ({ id: platform.id, label: platform.label })));

	let platformID: string | undefined = $state();

	const platform = $derived(platforms.find((entry) => entry.id === platformID) ?? platforms[0]);

	// Until the project's first error or log arrives, asked again every few seconds.
	const pollEvery = 3000;

	$effect(() => {
		if (setup.received) return;

		const timer = setInterval(() => void invalidate('app:project-setup'), pollEvery);

		return () => clearInterval(timer);
	});

	// The settings page is admins' only; for the others the path ends here.
	const crumbs = $derived([
		{ label: 'Projects', href: resolve('/(app)/projects') },
		...(canAdminister
			? [{ label: setup.name, href: resolve('/(app)/projects/[id=projectId]', { id }) }]
			: []),
		{ label: 'Installation', href: resolve('/(app)/projects/[id=projectId]/setup', { id }) }
	]);
</script>

<Breadcrumbs items={crumbs} />
<PageHeading title="Connect {setup.name}">
	{#snippet leading()}
		<BackButton fallback={crumbs[crumbs.length - 2].href} />
	{/snippet}
	{#snippet actions()}
		{#if platform}
			<CopyButton
				text={platform.agent_prompt}
				label="Copy for agent"
				title="Copies setup instructions to paste into a coding agent (Claude Code, Codex…)"
				copied="Instructions for your coding agent copied."
			/>
		{/if}
	{/snippet}
</PageHeading>
<p class="mb-4 text-muted">Pick what the app is written in, add these lines, and run it.</p>

<Tabs label="Platform" {tabs} param="platform" bind:selected={platformID}>
	{#snippet panel(tabID: string)}
		{@const shown = platforms.find((entry) => entry.id === tabID)}
		{#if shown}
			<ol class="flex list-decimal flex-col gap-5 pl-5">
				<li class="flex flex-col gap-2">
					<h2 class="font-semibold">Install the SDK</h2>
					<CodeBlock
						variant="command"
						code={shown.install}
						label="Install command"
						copyTitle="Copy the install command"
						copied="Command copied."
					/>
				</li>
				<li class="flex flex-col gap-2">
					<h2 class="font-semibold">Start it with your app</h2>
					<p class="text-xs text-muted">{shown.where}</p>
					<CodeBlock
						variant="snippet"
						code={shown.init}
						label="Setup code"
						copyTitle="Copy the code"
						copied="Code copied."
					/>
				</li>
				<li class="flex flex-col gap-2">
					<h2 class="font-semibold">
						Send a test error <span class="text-xs font-normal text-muted">optional</span>
					</h2>
					<CodeBlock
						variant="snippet"
						code={shown.test}
						label="Test line"
						copyTitle="Copy the test line"
						copied="Code copied."
					/>
				</li>
			</ol>
		{/if}
	{/snippet}
</Tabs>

<div class="mt-6" role="status">
	{#if setup.received}
		<Notice tone="success">
			{setup.name} is connected: its errors are on Issues, its logs on Logs.
		</Notice>
	{:else}
		<Notice tone="info">
			<p class="font-semibold">Waiting for {setup.name} to send its first error or log…</p>
			<p class="text-muted">Send the test error to check it now. This page updates by itself.</p>
			<p class="mt-2"><Button href={resolve('/(app)/projects')}>Do it later</Button></p>
		</Notice>
	{/if}
</div>
