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
		/** A link to another site: opened in a new tab, which gets no referrer and no opener. */
		external?: boolean;
		type?: 'button' | 'submit';
		/** The id of the form a submit button sends, when it sits outside it (a dialog's footer). */
		form?: string;
		disabled?: boolean;
		icon?: IconName;
		/** The name of an icon-only button, or a tooltip of one with text. */
		title?: string;
		onclick?: (event: MouseEvent) => void;
		/** A button that opens or closes something: whether it is open (aria-expanded). */
		expanded?: boolean;
		/** What it opens (aria-haspopup). */
		popup?: 'menu' | 'dialog';
		/** Its action runs: it ignores clicks but keeps the focus (disabled would drop it). */
		busy?: boolean;
		/** Why it does nothing now (Nothing changed): its tooltip; it ignores clicks, keeps the focus. */
		unavailable?: string;
		/** large: the one main action of a page (Add project), with larger corners. */
		size?: 'default' | 'large';
		/** Its text on one line, cut with "…" when its cell is narrower (a link in a fill column); title shows it whole. */
		truncate?: boolean;
		children?: Snippet;
	};

	let {
		variant = 'default',
		href,
		external = false,
		type = 'button',
		form,
		disabled = false,
		icon,
		title,
		onclick,
		expanded,
		popup,
		busy = false,
		unavailable,
		size = 'default',
		truncate = false,
		children
	}: Props = $props();

	const inert = $derived(busy || Boolean(unavailable));

	function click(event: MouseEvent) {
		if (inert) {
			event.preventDefault();

			return;
		}

		onclick?.(event);
	}

	// Boxed buttons share one focus: the border takes the tone, a ring of it glows; most glow
	// on hover too.
	const boxedBase =
		'border no-underline transition-[background-color,border-color,color,box-shadow] duration-150 hover:no-underline focus-visible:border-(--tone) focus-visible:ring-3 focus-visible:ring-(--tone)/40 focus-visible:outline-none';
	const boxed = `${boxedBase} not-disabled:hover:border-(--tone) not-disabled:hover:ring-3 not-disabled:hover:ring-(--tone)/22`;

	const variants: Record<ButtonVariant, string> = {
		default: `${boxed} [--tone:var(--color-muted)] border-border bg-surface text-text not-disabled:hover:bg-hover`,
		primary: `${boxed} [--tone:var(--color-accent)] border-accent bg-accent text-accent-text not-disabled:hover:bg-accent-hover`,
		danger: `${boxed} [--tone:var(--color-error)] border-border bg-surface text-error not-disabled:hover:bg-error-bg`,
		info: `${boxed} [--tone:var(--color-info)] border-info bg-info-bg text-info not-disabled:hover:bg-info-hover`,
		// Grey on the code background: the copy buttons of the MCP page's commands.
		neutral: `${boxed} [--tone:var(--color-muted)] border-border bg-code-bg text-text not-disabled:hover:bg-border`,
		// Opens a record beside the info buttons of a row (the issue list's Open).
		accent: `${boxed} [--tone:var(--color-accent)] border-accent bg-accent-soft text-accent not-disabled:hover:bg-accent not-disabled:hover:text-accent-text`,
		// The ⋯ of a row menu: no box until hovered or open.
		menu: `${boxedBase} [--tone:var(--color-muted)] border-transparent text-muted not-disabled:hover:border-muted not-disabled:hover:bg-hover aria-expanded:border-border aria-expanded:bg-hover aria-expanded:text-text`,
		link: 'focus-ring rounded-control text-accent not-disabled:hover:text-link-hover focus-visible:underline',
		// A secondary link, grey (a log's trace).
		muted: 'focus-ring rounded-control text-muted hover:underline focus-visible:underline',
		// The name of a table row that opens the record: the text's color, bold, on one line cut
		// with "…" when its cell is narrower (a fill column).
		title:
			'focus-ring max-w-full min-w-0 rounded-control text-left font-semibold text-text hover:underline focus-visible:underline',
		// Quiet: no box until hovered (the menu's toggle, Back).
		ghost: 'focus-ring text-muted not-disabled:hover:bg-bg not-disabled:hover:text-text'
	};

	const iconOnly = $derived(!children);
	const textual = $derived(variant === 'link' || variant === 'muted' || variant === 'title');

	// The box of a button that is not text-like: its size and corners.
	function boxClasses(): string {
		if (size === 'large' && !iconOnly) {
			return 'rounded-popup px-4.5 py-2.5 text-md font-semibold';
		}

		if (iconOnly) {
			const side = variant === 'ghost' ? 'size-7.5' : variant === 'menu' ? 'size-8' : 'size-8.5';

			return `rounded-control p-0 ${side}`;
		}

		if (variant === 'info' || variant === 'neutral') return 'h-9 rounded-control px-3';

		// A link is a little wider than a button, as in the design.
		return href ? 'rounded-control px-3 py-1.5' : 'rounded-control px-2.5 py-1.5';
	}

	const classes = $derived([
		'inline-flex cursor-pointer items-center gap-1.5 whitespace-nowrap',
		variant === 'title' ? 'justify-start' : 'justify-center',
		'disabled:cursor-not-allowed disabled:opacity-50 aria-disabled:cursor-not-allowed aria-disabled:opacity-50',
		'forced-colors:focus-visible:outline-2 forced-colors:focus-visible:outline-[Highlight]',
		variants[variant],
		truncate && 'max-w-full min-w-0',
		!textual && boxClasses()
	]);
</script>

{#snippet content()}
	{#if icon}<Icon name={icon} />{/if}
	{#if children}<span class={variant === 'title' || truncate ? 'truncate' : undefined}>
			{@render children()}
		</span>{/if}
{/snippet}

{#if href && !disabled}
	<!-- eslint-disable svelte/no-navigation-without-resolve -- callers pass resolved paths -->
	<a
		{href}
		target={external ? '_blank' : undefined}
		rel={external ? 'noopener noreferrer' : undefined}
		class={classes}
		{title}
		aria-label={iconOnly ? title : undefined}
	>
		{@render content()}
	</a>
	<!-- eslint-enable svelte/no-navigation-without-resolve -->
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
		{form}
		{disabled}
		onclick={click}
		aria-expanded={expanded}
		aria-haspopup={popup}
		aria-busy={busy || undefined}
		aria-disabled={inert || undefined}
		class={classes}
		title={unavailable ?? title}
		aria-label={iconOnly ? title : undefined}
	>
		{@render content()}
	</button>
{/if}
