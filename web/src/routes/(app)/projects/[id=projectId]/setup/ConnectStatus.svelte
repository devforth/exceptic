<!--
@component
Whether an app has connected to the project yet: waiting (with Do it later), or connected with
links to its issues and logs.
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import Button from '#lib/components/Button.svelte';
	import StatusCard from '#lib/components/StatusCard.svelte';
	import { withQuery } from '#lib/nav.ts';

	type Props = {
		name: string;
		connected: boolean;
		/** Where Do it later goes. */
		later: string;
	};

	let { name, connected, later }: Props = $props();
</script>

{#if connected}
	<StatusCard state="done" title="{name} is connected">
		Its errors appear on Issues and its logs on Logs as they arrive.
		{#snippet actions()}
			<Button href={withQuery(resolve('/(app)/issues'), { project: name })}>Open issues</Button>
			<Button href={withQuery(resolve('/(app)/logs'), { project: name })}>Open logs</Button>
		{/snippet}
	</StatusCard>
{:else}
	<StatusCard state="waiting" title="Waiting for {name} to connect…">
		Most SDKs check in as soon as the app starts or serves its first request. To check right away,
		send the test error. This page updates by itself.
		{#snippet actions()}
			<Button href={later}>Do it later</Button>
		{/snippet}
	</StatusCard>
{/if}
