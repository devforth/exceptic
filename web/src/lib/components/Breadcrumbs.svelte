<!--
@component
The path above a page's title, "Projects / backend": the ancestors muted, the last item the
page itself. Esc follows the item before it (one level up), unless a field has the focus or a
dialog, menu or dropdown is open (#lib/escape.ts). It is decided on keydown and followed on
keyup: Firefox stops loading a page on Esc, which would cancel a navigation started on keydown.
-->
<script lang="ts">
	import { escapeGoesBack } from '#lib/escape.ts';

	type Crumb = { label: string; href: string };

	let { items }: { items: Crumb[] } = $props();

	let parent: HTMLAnchorElement | undefined = $state();
	let backOnKeyup = false;

	function keydown(event: KeyboardEvent) {
		backOnKeyup = parent !== undefined && escapeGoesBack(event);

		if (backOnKeyup) event.preventDefault();
	}

	// A click, so the app's router follows the link as it follows any other.
	function keyup(event: KeyboardEvent) {
		if (event.key === 'Escape' && backOnKeyup) parent?.click();

		backOnKeyup = false;
	}

	const linkClasses = 'rounded-control text-muted no-underline focus-ring hover:text-text';
</script>

<svelte:document onkeydown={keydown} onkeyup={keyup} />

<nav aria-label="Breadcrumb" class="mb-1.5">
	<ol class="flex flex-wrap items-center gap-1.5 text-label">
		{#each items as item, index (item.href)}
			<li class="flex items-center gap-1.5">
				{#if index > 0}<span class="text-muted opacity-50" aria-hidden="true">/</span>{/if}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- callers pass resolved paths -->
				{#if index === items.length - 1}
					<a href={item.href} aria-current="page" class={[linkClasses, 'font-semibold text-text']}>
						{item.label}
					</a>
				{:else if index === items.length - 2}
					<a bind:this={parent} href={item.href} title="Back (Esc)" class={linkClasses}>
						{item.label}
					</a>
				{:else}
					<a href={item.href} class={linkClasses}>{item.label}</a>
				{/if}
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			</li>
		{/each}
	</ol>
</nav>
