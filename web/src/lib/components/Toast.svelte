<!--
@component
The live region of #lib/toast. Mount it once, in the root layout: it stays in the page so
screen readers announce each message.
-->
<script lang="ts">
	import { toast } from '#lib/toast.svelte.ts';
</script>

<div role="status" aria-live="polite" class="fixed right-5 bottom-5 z-50">
	{#if toast.message}
		<p
			class="max-w-[min(360px,calc(100vw-40px))] rounded-note border border-info bg-info-bg px-4 py-3 text-info shadow-toast"
		>
			{toast.message}
		</p>
	{/if}
</div>
