<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { post } from '#lib/api/client.ts';
	import ActionList from '#lib/components/ActionList.svelte';
	import ActionRow from '#lib/components/ActionRow.svelte';
	import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
	import { timeAgo } from '#lib/format.ts';
	import { pauseConfirmation } from '#lib/projects.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import BackButton from '#lib/components/BackButton.svelte';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import Badge from '#lib/components/Badge.svelte';
	import { titleClasses } from '#lib/components/heading.ts';
	import CodeBlock from '#lib/components/CodeBlock.svelte';
	import CopyButton from '#lib/components/CopyButton.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import Step from '#lib/components/Step.svelte';
	import Steps from '#lib/components/Steps.svelte';
	import Tabs from '#lib/components/Tabs.svelte';
	import ConnectStatus from './ConnectStatus.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const setup = $derived(data.setup);
	const platforms = $derived(setup.platforms ?? []);
	const id = $derived(setup.id);
	const tabs = $derived(platforms.map((platform) => ({ id: platform.id, label: platform.label })));

	let platformID: string | undefined = $state();

	const platform = $derived(platforms.find((entry) => entry.id === platformID) ?? platforms[0]);

	// Until an app sends anything with the project's key, asked again every few seconds.
	const pollEvery = 3000;

	$effect(() => {
		if (setup.connected) return;

		const timer = setInterval(() => void invalidate('app:project-setup'), pollEvery);

		return () => clearInterval(timer);
	});

	let resetOpen = $state(false);

	// When the DSN last changed: its reset, else the project's creation.
	const keyAge = $derived(
		setup.key_changed_at
			? `Last reset ${timeAgo(setup.key_changed_at)}`
			: `Unchanged since the project was added ${timeAgo(setup.created_at)}`
	);

	let pauseOpen = $state(false);
	const pause = $derived(pauseConfirmation(setup.name));

	async function setActive(active: boolean) {
		await post('set_project_active', { project: String(id), active });
		toast.show(
			active ? `Recording ${setup.name} again.` : `${setup.name} paused: nothing is stored.`
		);
		await invalidate('app:project-setup');
	}

	async function resetKey() {
		await post('reset_project_key', { project: id });
		toast.show('New DSN: give it to the apps of this project.');
		await invalidate('app:project-setup');
	}

	// The settings page is admins' only; for the others the path ends here.
	const crumbs = $derived([
		{ label: 'Projects', href: resolve('/(app)/projects') },
		{ label: setup.name, href: resolve('/(app)/projects/[id=projectId]', { id }) },
		{ label: 'Installation', href: resolve('/(app)/projects/[id=projectId]/setup', { id }) }
	]);
</script>

<Breadcrumbs items={crumbs} />
<!-- The heading spans the steps, so Copy for agent ends where they do. -->
<div class="max-w-guide">
	<PageHeading title="Connect {setup.name}">
		{#snippet leading()}
			<BackButton fallback={crumbs[crumbs.length - 2].href} />
		{/snippet}
		{#snippet heading()}
			<h1 class={[titleClasses, 'flex items-center gap-2.5']}>
				Connect <Badge tone="project" label={setup.name} size="heading" />
			</h1>
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
		{#snippet description()}Pick what the app is written in, add these lines, and run it.{/snippet}
	</PageHeading>
</div>

<Tabs label="Platform" {tabs} param="platform" bind:selected={platformID}>
	{#snippet panel(tabID: string)}
		{@const shown = platforms.find((entry) => entry.id === tabID)}
		{#if shown}
			<Steps>
				<Step title="Install the SDK">
					<CodeBlock
						variant="snippet"
						code={shown.install}
						label="Install command"
						copyTitle="Copy the install command"
						copied="Command copied."
					/>
				</Step>
				<Step title="Start it with your app" note={shown.where}>
					<CodeBlock
						variant="snippet"
						code={shown.init}
						label="Setup code"
						copyTitle="Copy the code"
						copied="Code copied."
					/>
				</Step>
				<Step title="Send a test error" optional>
					<CodeBlock
						variant="snippet"
						code={shown.test}
						label="Test line"
						copyTitle="Copy the test line"
						copied="Code copied."
					/>
				</Step>
			</Steps>
		{/if}
	{/snippet}
</Tabs>

<ConnectStatus name={setup.name} connected={setup.connected} later={resolve('/(app)/projects')} />

<div class="mt-6 max-w-guide">
	<ActionList title="Connection">
		<ActionRow
			icon="token"
			label="Reset DSN"
			description={keyAge}
			onclick={() => (resetOpen = true)}
		/>
		{#if setup.paused}
			<ActionRow
				icon="resume"
				label="Resume recording"
				description="Its apps are answered, nothing is stored"
				status={{ label: 'Paused', tone: 'muted' }}
				onclick={() => void setActive(true)}
			/>
		{:else}
			<ActionRow
				icon="pause"
				label="Pause recording"
				description="What its apps send is stored"
				status={{ label: 'Active', tone: 'success' }}
				onclick={() => (pauseOpen = true)}
			/>
		{/if}
	</ActionList>
</div>
<ConfirmDialog
	bind:open={resetOpen}
	title="Reset DSN"
	message="Give {setup.name} a new DSN? Apps still using the current one are refused until they get the new one."
	confirmLabel="Reset DSN"
	onconfirm={resetKey}
/>
<ConfirmDialog bind:open={pauseOpen} {...pause} onconfirm={() => setActive(false)} />
