<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { post } from '#lib/api/client.ts';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import Button from '#lib/components/Button.svelte';
	import ConfirmButton from '#lib/components/ConfirmButton.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import { FormState } from '#lib/form.svelte.ts';
	import { deleteConfirmation, deleteConnection } from '#lib/notifications.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import ConnectionForm from '../ConnectionForm.svelte';
	import RetryNowButton from '../RetryNowButton.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const connection = $derived(data.connection);
	const test = new FormState();

	async function remove() {
		await deleteConnection(connection.id);
		await goto(resolve('/(app)/notifications'));
	}

	async function sendTest() {
		if (await test.submit(() => post('send_test_message', { connection_id: connection.id }))) {
			toast.show('Test message sent.');
		}
	}
</script>

<Breadcrumbs
	items={[
		{ label: 'Notifications', href: resolve('/(app)/notifications') },
		{
			label: connection.name,
			href: resolve('/(app)/notifications/[id=connectionId]', { id: connection.id })
		}
	]}
/>
<PageHeading title={connection.name}>
	{#snippet actions()}
		<ConfirmButton icon="delete" confirm={deleteConfirmation(connection.name)} action={remove}>
			Delete
		</ConfirmButton>
	{/snippet}
</PageHeading>

{#key connection.id}
	<ConnectionForm defaults={data.defaults} projects={data.projects} {connection} />
{/key}

{#if connection.retrying > 0 || (connection.type === 'email' && connection.confirmed)}
	<div class="mt-6 flex max-w-3xl flex-col gap-3 border-t border-border pt-4">
		<div class="flex flex-wrap gap-2">
			{#if connection.retrying > 0}
				<RetryNowButton connectionId={connection.id} waiting={connection.retrying} />
			{/if}
			<!-- Chats are tested from their fields, before saving. -->
			{#if connection.type === 'email' && connection.confirmed}
				<Button icon="send" busy={test.busy} onclick={sendTest}>Send test message</Button>
			{/if}
		</div>
		{#if test.error}<Notice tone="error">{test.error}</Notice>{/if}
	</div>
{/if}
