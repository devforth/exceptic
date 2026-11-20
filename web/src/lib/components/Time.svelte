<!--
@component
A time as "5m ago", the exact UTC time in its tooltip.
-->
<script lang="ts">
	import { exactTime, timeAgo } from '#lib/format.ts';

	let { at }: { at: string } = $props();
</script>

<time datetime={at} title={exactTime(at)} class="whitespace-nowrap">{timeAgo(at)}</time>
