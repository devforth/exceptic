<!--
@component
An item of the menu: a link, a button (with onclick), a switch (onclick and checked: Dark mode,
a Switch at its end) or the submit button of a form (Log out, `danger`: in the error color). In a collapsed menu or under
800px wide it shows its icon only, its count in a corner, a switch's icon in the accent when on.
-->
<script lang="ts">
	import type { IconName } from '#lib/icons.ts';
	import Icon from './Icon.svelte';
	import Switch from './Switch.svelte';

	type Props = {
		label: string;
		icon: IconName;
		/** A link; without it and onclick, a submit button. */
		href?: string;
		onclick?: () => void;
		title?: string;
		active?: boolean;
		count?: number;
		/** What the count is, for screen readers: "7 unresolved". */
		countLabel?: string;
		collapsed?: boolean;
		/** With onclick: a switch, on or off. */
		checked?: boolean;
		/** In the error color: Log out. */
		danger?: boolean;
	};

	let {
		label,
		icon,
		href,
		onclick,
		title,
		active = false,
		count,
		countLabel,
		collapsed = false,
		checked,
		danger = false
	}: Props = $props();

	const classes = $derived([
		'relative flex w-full items-center gap-2.5 rounded-control px-2.5 py-2 text-left text-sm no-underline',
		'cursor-pointer hover:no-underline',
		danger ? 'text-error hover:bg-error-bg' : 'text-muted hover:bg-bg hover:text-text',
		'focus-ring [--focus-offset:-2px]',
		'max-sidebar:w-auto max-sidebar:px-3 max-sidebar:py-[9px]',
		collapsed && 'sidebar:justify-center sidebar:p-2',
		active && 'bg-bg font-semibold text-text forced-colors:outline forced-colors:outline-1'
	]);

	// Beside the label in the full menu, on the icon's corner otherwise (class names in
	// full: Tailwind finds them in the source).
	const countClasses = $derived([
		'rounded-full bg-error-bg text-center font-semibold text-error forced-colors:border',
		collapsed
			? 'absolute top-0 right-0 px-1 text-3xs leading-[15px] max-sidebar:top-px max-sidebar:right-0.5'
			: 'min-w-[22px] px-1.5 py-px text-xs leading-[18px] max-sidebar:absolute max-sidebar:top-px max-sidebar:right-0.5 max-sidebar:min-w-0 max-sidebar:px-1 max-sidebar:py-0 max-sidebar:text-3xs max-sidebar:leading-[15px] sidebar:ml-auto'
	]);

	const shownCount = $derived(count === undefined ? '' : count > 999 ? '999+' : String(count));
</script>

{#snippet content()}
	{#if checked}
		<!-- On, while only the icon shows: the icon in the accent. -->
		<span class={['flex max-sidebar:text-accent', collapsed && 'sidebar:text-accent']}>
			<Icon name={icon} />
		</span>
	{:else}
		<Icon name={icon} />
	{/if}
	<span class={['max-sidebar:sr-only', collapsed && 'sidebar:sr-only']}>{label}</span>
	{#if checked !== undefined}
		<span class={['ml-auto flex max-sidebar:hidden', collapsed && 'sidebar:hidden']}>
			<Switch {checked} decorative />
		</span>
	{/if}
	{#if count}
		<b class={countClasses} aria-hidden="true">{shownCount}</b>
		{#if countLabel}<span class="sr-only">{countLabel}</span>{/if}
	{/if}
{/snippet}

{#if href}
	<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the menu passes resolved paths -->
	<a {href} class={classes} title={title ?? label} aria-current={active ? 'page' : undefined}>
		{@render content()}
	</a>
{:else if onclick}
	<button
		type="button"
		class={classes}
		title={title ?? label}
		role={checked === undefined ? undefined : 'switch'}
		aria-checked={checked}
		{onclick}
	>
		{@render content()}
	</button>
{:else}
	<button type="submit" class={classes} title={title ?? label}>{@render content()}</button>
{/if}
