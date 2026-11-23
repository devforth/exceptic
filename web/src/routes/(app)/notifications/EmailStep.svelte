<!--
@component
The address of a new email connection: entered, then confirmed with the code mailed to it
(an address confirmed before needs none). Its own form, so Enter verifies or confirms instead
of saving the connection; the rest of the connection form opens once it is verified.
-->
<script lang="ts">
	import { help, limits } from '#lib/api/catalog.gen.ts';
	import { post, type Out } from '#lib/api/client.ts';
	import Button from '#lib/components/Button.svelte';
	import FormError from '#lib/components/FormError.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import TextField from '#lib/components/TextField.svelte';
	import Time from '#lib/components/Time.svelte';
	import { FormState } from '#lib/form.svelte.ts';
	import { emailStepAfter, type EmailStepState } from '#lib/notifications.ts';
	import DuplicateNotice from './DuplicateNotice.svelte';

	type Props = {
		/** The address as typed (the name's placeholder follows it). */
		typed: string;
		/** Where the step is; kept by the form, so switching the type away and back keeps it. */
		progress: EmailStepState;
		/** The save's problem with the address. */
		error?: string;
	};

	let { typed = $bindable(), progress = $bindable(), error }: Props = $props();

	let code = $state('');
	const request = new FormState();

	async function send(run: () => Promise<Out<'start_email_verification'>>) {
		let answer: Out<'start_email_verification'> | undefined;

		if ((await request.submit(async () => (answer = await run()))) && answer) {
			progress = emailStepAfter(answer);
			code = '';
		}
	}

	function verify(event: SubmitEvent) {
		event.preventDefault();
		void send(() => post('start_email_verification', { email: typed }));
	}

	function confirm(event: SubmitEvent) {
		event.preventDefault();
		void send(() => post('confirm_email', { email: progress.address, code: code.trim() }));
	}

	function sendAgain() {
		void send(() => post('start_email_verification', { email: progress.address }));
	}

	function change() {
		progress = { step: 'input', address: '', notice: '' };
	}
</script>

<div class="flex flex-col gap-2">
	{#if progress.step === 'input'}
		<form onsubmit={verify}>
			<TextField
				label="Email address"
				name="email"
				type="email"
				bind:value={typed}
				required
				placeholder="team@example.com"
				autocomplete="email"
				hint={help.fields.Notifications['Email address']}
				description="We send a confirmation code to this address. Addresses confirmed before need no new code."
				error={request.fields.email ?? error}
			>
				{#snippet action()}
					<Button type="submit" variant="primary" busy={request.busy}>Verify</Button>
				{/snippet}
			</TextField>
		</form>
	{:else}
		<p class="flex flex-wrap items-center gap-2">
			<b>{progress.address}</b>
			{#if progress.step === 'verified'}
				<span class="text-sm text-success">
					✓ confirmed{#if progress.verifiedAt}&nbsp;<Time at={progress.verifiedAt} />{/if}
				</span>
			{/if}
		</p>
		{#if progress.step === 'code'}
			<form onsubmit={confirm}>
				<TextField
					label="Confirmation code"
					name="code"
					variant="code"
					bind:value={code}
					required
					maxlength={limits.verificationCode}
					placeholder="123456"
					autocomplete="one-time-code"
					error={request.fields.code}
				>
					{#snippet action()}
						<Button type="submit" variant="primary" busy={request.busy}>Confirm</Button>
					{/snippet}
				</TextField>
			</form>
		{/if}
		<span class="flex flex-wrap gap-4 text-sm">
			{#if progress.step === 'code'}
				<Button variant="link" onclick={sendAgain}>Send the code again</Button>
			{/if}
			<Button variant="link" onclick={change}>Change email</Button>
		</span>
	{/if}
	{#if request.duplicate}<DuplicateNotice duplicate={request.duplicate} />{/if}
	{#if progress.notice}<Notice tone="success">{progress.notice}</Notice>{/if}
	<FormError form={request} />
</div>
