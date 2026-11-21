<!--
@component
"Copy for agent": copies the MCP request that has a coding agent fetch an issue (the server
writes it, mcp_reference), as notifications carry it.
-->
<script lang="ts">
	import CopyButton from './CopyButton.svelte';

	let { request }: { request: string } = $props();
</script>

<CopyButton
	text={request}
	label="Copy for agent"
	title="Copy for agent: the MCP request that makes a coding agent fetch this issue"
	copied="Request for the agent copied to clipboard."
/>
