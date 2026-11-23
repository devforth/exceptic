<script lang="ts">
	import './layout.css';
	import Toast from '#lib/components/Toast.svelte';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
</script>

{@render children()}
<Toast position="bottom-right" />
