<!--
@component
"Send test message" under a chat's fields: sends with the fields as typed, before they are
saved, and says beside it what happened. It works once the fields it needs are filled.
-->
<script lang="ts">
	import { ApiError } from '#lib/api/client.ts';
	import Button from '#lib/components/Button.svelte';

	type Props = {
		ready: boolean;
		test: () => Promise<{ sent: boolean; message: string }>;
	};

	let { ready, test }: Props = $props();

	let busy = $state(false);
	let result: { sent: boolean; message: string } | undefined = $state();

	async function run() {
		busy = true;
		result = undefined;

		try {
			result = await test();
		} catch (failure) {
			result = {
				sent: false,
				message: failure instanceof ApiError ? failure.message : 'Something went wrong.'
			};
		} finally {
			busy = false;
		}
	}
</script>

<div class="flex flex-wrap items-center gap-2.5">
	<Button icon="send" disabled={!ready} {busy} onclick={run}>Send test message</Button>
	<span role="status" class={['text-xs', result?.sent ? 'text-success' : 'text-error']}>
		{result?.message ?? ''}
	</span>
</div>
