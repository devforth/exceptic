<!--
@component
A user's role as a badge: the roles an administrator gives are plain, the one superadmin
stands out. A look only, never a permission.
-->
<script lang="ts">
	import Badge from './Badge.svelte';
	import { isAssignable } from './role.ts';

	let { role }: { role: string } = $props();
</script>

<Badge tone={isAssignable(role) ? 'plain' : 'info'} label={role} />
