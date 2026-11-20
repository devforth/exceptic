<!--
@component
How full something is: a bar filled to percent, read as a meter by screen readers. The width
is set through the DOM (style: directive), which the CSP allows, unlike a style attribute.
-->
<script lang="ts">
	type Props = { percent: number; label: string };

	let { percent, label }: Props = $props();

	const filled = $derived(Math.min(100, Math.max(0, percent)));
</script>

<div
	role="meter"
	aria-label={label}
	aria-valuenow={filled}
	aria-valuemin={0}
	aria-valuemax={100}
	class="h-2 w-full max-w-[520px] overflow-hidden rounded-full bg-code-bg forced-colors:border"
>
	<div class="h-full rounded-full bg-accent" style:width="{filled}%"></div>
</div>
