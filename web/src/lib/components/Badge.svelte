<!--
@component
A short label: a level or state filled in its tone, or a project outlined in its color (from
the session, or `color` for one not saved yet; set through the style: directive, which the CSP
allows). With an href it is a link; a mark is a sign before the label (an issue status), hidden
from screen readers.
-->
<script lang="ts">
	import { page } from '$app/state';
	import type { BadgeTone } from './badge.ts';

	type Props = {
		tone: BadgeTone;
		/** The text; for a project, its name. */
		label: string;
		href?: string;
		title?: string;
		mark?: string;
		/** project: this color instead of the saved one (a preview), #rrggbb. */
		color?: string;
	};

	let { tone, label, href, title, mark, color: chosen }: Props = $props();

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
	const color = $derived(
		project ? (chosen ?? page.data.session?.project_colors?.[project]) : undefined
	);
</script>

{#snippet text()}{#if mark}<span class="mr-1" aria-hidden="true">{mark}</span>{/if}{label}{/snippet}

{#if href}
	<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- callers pass resolved paths -->
	<a {href} class={classes} {title} data-project={project} style:--project-color={color}>
		{@render text()}
	</a>
{:else}
	<span class={classes} {title} data-project={project} style:--project-color={color}>
		{@render text()}
	</span>
{/if}
