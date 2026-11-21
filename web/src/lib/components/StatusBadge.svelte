<!--
@component
An issue status: its sign and name in its tone, what it means in the tooltip. With an href it
is a link (the issue list of that status).
-->
<script lang="ts">
	import Badge from './Badge.svelte';
	import { statusLook, statusTitle } from './issueStatus.ts';

	let { status, href }: { status: string; href?: string } = $props();

	const look = $derived(statusLook(status));
</script>

<Badge tone={look.tone} mark={look.mark} label={status} {href} title={statusTitle(status)} />
