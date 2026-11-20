<!--
@component
A button, or a link that looks like one when it has an href. Without children it shows
only its icon, and title names it (tooltip and aria-label).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { ButtonVariant } from './button.ts';
	import type { IconName } from '#lib/icons.ts';
	import Icon from './Icon.svelte';

	type Props = {
		variant?: ButtonVariant;
		href?: string;
		type?: 'button' | 'submit';
		disabled?: boolean;
		icon?: IconName;
		/** The name of an icon-only button, or a tooltip of one with text. */
		title?: string;
		/** Form fields of a submit button. */
		name?: string;
		value?: string;
		onclick?: (event: MouseEvent) => void;
		/** A button that opens or closes something: whether it is open (aria-expanded). */
		expanded?: boolean;
		children?: Snippet;
	};

	let {
		variant = 'default',
		href,
		type = 'button',
		disabled = false,
		icon,
		title,
		name,
		value,
		onclick,
		expanded,
		children
	}: Props = $props();

	// Boxed buttons share one hover and focus: the border takes the tone, a ring of it glows.
	const boxed =
		'rounded-control border no-underline transition-[background-color,border-color,color,box-shadow] duration-150 hover:no-underline not-disabled:hover:border-(--tone) not-disabled:hover:ring-3 not-disabled:hover:ring-(--tone)/22 focus-visible:border-(--tone) focus-visible:ring-3 focus-visible:ring-(--tone)/40 focus-visible:outline-none';

	const variants: Record<ButtonVariant, string> = {
		default: `${boxed} [--tone:var(--color-muted)] border-border bg-surface text-text not-disabled:hover:bg-hover`,
		primary: `${boxed} [--tone:var(--color-accent)] border-accent bg-accent text-accent-text not-disabled:hover:bg-accent-hover`,
		danger: `${boxed} [--tone:var(--color-error)] border-border bg-surface text-error not-disabled:hover:bg-error-bg`,
		info: `${boxed} [--tone:var(--color-info)] border-info bg-info-bg text-info not-disabled:hover:bg-info-hover`,
		link: 'focus-ring rounded-control text-accent hover:underline focus-visible:underline',
		// Quiet: no box until hovered (the menu's toggle).
		ghost:
			'focus-ring rounded-control text-muted not-disabled:hover:bg-bg not-disabled:hover:text-text'
	};

	const iconOnly = $derived(!children);

	const classes = $derived([
		'inline-flex cursor-pointer items-center justify-center gap-1.5 whitespace-nowrap',
		'disabled:cursor-not-allowed disabled:opacity-50 aria-disabled:cursor-not-allowed aria-disabled:opacity-50',
		'forced-colors:focus-visible:outline-2 forced-colors:focus-visible:outline-[Highlight]',
		variants[variant],
		variant !== 'link' &&
			(iconOnly ? 'size-8.5 p-0' : variant === 'info' ? 'h-9 px-3' : 'px-3 py-1.5')
	]);
</script>

{#snippet content()}
	{#if icon}<Icon name={icon} />{/if}
	{#if children}<span>{@render children()}</span>{/if}
{/snippet}

{#if href && !disabled}
	<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- callers pass resolved paths -->
	<a {href} class={classes} {title} aria-label={iconOnly ? title : undefined}>
		{@render content()}
	</a>
{:else if href}
	<a
		role="link"
		aria-disabled="true"
		class={classes}
		{title}
		aria-label={iconOnly ? title : undefined}
	>
		{@render content()}
	</a>
{:else}
	<button
		{type}
		{disabled}
		{name}
		{value}
		{onclick}
		aria-expanded={expanded}
		class={classes}
		{title}
		aria-label={iconOnly ? title : undefined}
	>
		{@render content()}
	</button>
{/if}
