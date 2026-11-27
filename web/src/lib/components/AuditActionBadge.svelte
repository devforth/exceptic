<!--
@component
The action of an audit log entry as a badge: a create green, an edit blue, a delete red, the
other actions (pause, reset DSN, …) plain.
-->
<script lang="ts">
	import { actionLabel, actionTone } from '#lib/audit.ts';
	import Badge from './Badge.svelte';

	let { action, href }: { action: string; href?: string } = $props();
</script>

<Badge tone={actionTone(action)} label={actionLabel(action)} {href} />
