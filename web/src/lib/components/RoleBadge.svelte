<!--
@component
A user's role as a badge: an admin stands out, a user is plain. A look only, never a
permission.
-->
<script lang="ts">
	import Badge from './Badge.svelte';

	let { role }: { role: string } = $props();
</script>

<Badge tone={role === 'admin' ? 'info' : 'plain'} label={role} />
