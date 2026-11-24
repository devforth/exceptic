<!--
@component
A short label: a level or role filled in its tone, or a project outlined in its color (from
the session, or `color` for one not saved yet; set through the style: directive, which the CSP
allows), with `size="heading"` beside a page's title. With an href it is a link; with `remove` the whole badge
links to where it goes away (a filter's value), a × inside it. A state (on, off) is a StatusText.
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
		/** project: this color instead of the saved one (a preview), #rrggbb. */
		color?: string;
		/** heading: a project beside a page's title (Connect backend). */
		size?: 'default' | 'heading';
		/** A × inside the badge: the address without it and the ×'s accessible name. */
		remove?: { href: string; label: string };
	};

	let { tone, label, href, title, color: chosen, size = 'default', remove }: Props = $props();

	const filled = 'px-[7px] py-px align-[2px] lowercase';

	const tones: Record<BadgeTone, string> = {
		error: `${filled} bg-error-badge text-error`,
		warning: `${filled} bg-warning-bg text-warning`,
		info: `${filled} bg-info-bg text-info`,
		debug: `${filled} bg-debug-bg text-debug`,
		plain: `${filled} bg-code-bg text-text`,
		project:
			'border border-project-text px-[7px] align-[1px] text-project-text first-letter:uppercase'
	};

	const classes = $derived([
		'inline-block rounded-full font-semibold whitespace-nowrap forced-colors:border',
		size === 'heading' ? 'text-h2' : 'text-2xs',
		tones[tone],
		(href || remove) && 'no-underline hover:no-underline hover:brightness-92',
		remove && 'group inline-flex items-center gap-0.5 pr-[3px] focus-ring'
	]);

	const project = $derived(tone === 'project' ? label : undefined);
	const color = $derived(
		project ? (chosen ?? page.data.session?.project_colors?.[project]) : undefined
	);
</script>

<!-- A link with href or remove (callers pass resolved paths), else plain text; a removable badge
     is a link as a whole, its × a sign of what it does. -->
<svelte:element
	this={href || remove ? 'a' : 'span'}
	href={remove?.href ?? href}
	class={classes}
	title={remove ? 'Remove' : title}
	aria-label={remove?.label}
	data-project={project}
	style:--project-color={color}
>
	<!-- A flex badge (remove) has no first letter of its own: its label carries the project's capital. -->
	{#if remove}<span class={[tone === 'project' && 'inline-block first-letter:uppercase']}>
			{label}
		</span>{:else}{label}{/if}
	{#if remove}
		<span
			class="inline-flex size-3.5 items-center justify-center rounded-full text-xs leading-none opacity-70 group-hover:bg-text/10 group-hover:opacity-100"
			aria-hidden="true"
		>
			×
		</span>
	{/if}
</svelte:element>
