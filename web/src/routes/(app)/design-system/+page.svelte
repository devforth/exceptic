<script lang="ts">
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const Page = $derived(data.page);
</script>

<Page />
