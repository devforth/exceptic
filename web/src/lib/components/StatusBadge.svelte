<!--
@component
An issue status: its sign and name as bold text in its tone, not a pill; what it means in the
tooltip. With an href it is a link (the issue list of that status).
-->
<script lang="ts">
	import { statusLook, statusTitle, type StatusTextTone } from './issueStatus.ts';

	let { status, href }: { status: string; href?: string } = $props();

	const look = $derived(statusLook(status));

	const tones: Record<StatusTextTone, string> = {
		error: 'text-error',
		success: 'text-success',
		muted: 'text-muted',
		plain: 'text-text'
	};

	const classes = $derived([
		'text-xs font-semibold whitespace-nowrap',
		tones[look.tone],
		href && 'focus-ring rounded-control'
	]);
</script>

{#snippet text()}
	{#if look.mark}<span
			class={[
				'mr-0.5 inline-block w-[1.1em] text-center leading-none font-bold',
				look.smallMark && 'text-[.8em]'
			]}
			aria-hidden="true"
		>
			{look.mark}
		</span>{/if}{status}
{/snippet}

{#if href}
	<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- callers pass resolved paths -->
	<a {href} class={classes} title={statusTitle(status)}>{@render text()}</a>
{:else}
	<span class={classes} title={statusTitle(status)}>{@render text()}</span>
{/if}
