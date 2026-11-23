<!--
The login, outside the app's shell: no menu and no session request. The server decides where
to go after it (a local path only); a full load opens that page with a fresh session.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { send } from '#lib/api/client.ts';
	import Card from '#lib/components/Card.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageTitle from '#lib/components/PageTitle.svelte';
	import SubmitButton from '#lib/components/SubmitButton.svelte';
	import TextField from '#lib/components/TextField.svelte';
	import { FormState } from '#lib/form.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const form = new FormState();
	let username = $state('');
	let password = $state('');
	let element: HTMLFormElement | undefined = $state();

	function focus(name: string) {
		element?.querySelector<HTMLInputElement>(`input[name="${name}"]`)?.focus();
	}

	onMount(() => focus('username'));

	async function submit(event: SubmitEvent) {
		event.preventDefault();

		let next = '';
		const loggedIn = await form.submit(async () => {
			({ next } = await send('login', {
				username,
				password,
				next: page.url.searchParams.get('next') ?? ''
			}));
		});

		if (loggedIn) {
			location.assign(next);

			return;
		}

		password = '';
		focus('password');
	}
</script>

<PageTitle title="Log in" />

<main class="mx-auto w-full max-w-[340px] px-4 pt-[12vh] pb-16">
	<Card>
		<h1 class="text-h1 font-bold wrap-break-word">{data.state.app_name}</h1>
		{#if data.state.disabled}
			<Notice tone="info">{data.state.disabled}</Notice>
		{:else}
			<form bind:this={element} onsubmit={submit} class="flex flex-col gap-3.5">
				{#if form.error}
					<div role="alert"><Notice tone="error">{form.error}</Notice></div>
				{/if}
				<TextField
					label="Username"
					name="username"
					bind:value={username}
					autocomplete="username"
					required
				/>
				<TextField
					label="Password"
					name="password"
					type="password"
					bind:value={password}
					autocomplete="current-password"
					required
				/>
				<SubmitButton {form}>Log in</SubmitButton>
			</form>
		{/if}
	</Card>
</main>
