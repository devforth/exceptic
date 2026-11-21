<!--
@component
"Retry now (N)": the messages of a connection that wait for a retry are sent again at once,
for when the cause was fixed outside this server.
-->
<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { ApiError, post } from '#lib/api/client.ts';
	import Button from '#lib/components/Button.svelte';
	import { toast } from '#lib/toast.svelte.ts';

	let { connectionId, waiting }: { connectionId: string; waiting: number } = $props();

	let busy = $state(false);

	async function retry() {
		busy = true;

		try {
			await post('retry_connection', { connection_id: connectionId });
			toast.show('The waiting messages are being sent again now.');
			await invalidateAll();
		} catch (failure) {
			toast.show(failure instanceof ApiError ? failure.message : 'Something went wrong.');
		} finally {
			busy = false;
		}
	}
</script>

<Button
	icon="retry"
	{busy}
	title="{waiting} message(s) wait for a retry: send them again now"
	onclick={retry}
>
	Retry now ({waiting})
</Button>
