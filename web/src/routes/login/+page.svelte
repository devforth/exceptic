<!--
The login, outside the app's shell: no menu and no session request. The server decides where
to go after it (a local path only); a full load opens that page with a fresh session.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { send } from '#lib/api/client.ts';
	import Card from '#lib/components/Card.svelte';
	import { titleClasses } from '#lib/components/heading.ts';
	import { formClasses } from '#lib/components/field.ts';
	import FormActions from '#lib/components/FormActions.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageTitle from '#lib/components/PageTitle.svelte';
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
		<h1 class={titleClasses}>{data.state.app_name}</h1>
		{#if data.state.disabled}
			<Notice tone="info">{data.state.disabled}</Notice>
		{:else}
			<form bind:this={element} onsubmit={submit} class={formClasses}>
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
				<FormActions {form} stretch>Log in</FormActions>
			</form>
		{/if}
	</Card>
</main>
