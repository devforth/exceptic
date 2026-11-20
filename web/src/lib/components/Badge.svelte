<!--
@component
A short label: a level or state filled in its tone, or a project outlined in its color
(/project-colors.css sets it on [data-project]). With an href it is a link.
-->
<script lang="ts">
	import type { BadgeTone } from './badge.ts';

	type Props = {
		tone: BadgeTone;
		/** The text; for a project, its name. */
		label: string;
		href?: string;
		title?: string;
	};

	let { tone, label, href, title }: Props = $props();

	const filled = 'px-[7px] py-px align-[2px] lowercase';

	const tones: Record<BadgeTone, string> = {
		error: `${filled} bg-error-badge text-error`,
		warning: `${filled} bg-warning-bg text-warning`,
		info: `${filled} bg-info-bg text-info`,
		debug: `${filled} bg-debug-bg text-debug`,
		plain: `${filled} bg-code-bg text-text`,
		success: `${filled} bg-success-bg text-success`,
		muted: `${filled} bg-debug-bg text-muted`,
		project:
			'border border-project-text px-[7px] align-[1px] text-project-text first-letter:uppercase'
	};

	const classes = $derived([
		'inline-block rounded-full text-2xs font-semibold whitespace-nowrap forced-colors:border',
		tones[tone],
		href && 'no-underline hover:no-underline hover:brightness-92'
	]);

	const project = $derived(tone === 'project' ? label : undefined);
</script>

{#if href}
	<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- callers pass resolved paths -->
	<a {href} class={classes} {title} data-project={project}>{label}</a>
{:else}
	<span class={classes} {title} data-project={project}>{label}</span>
{/if}
