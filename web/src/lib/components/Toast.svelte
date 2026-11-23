<!--
@component
The toasts of #lib/toast, stacked in one corner (`position`). Mount it once, in the root layout:
the live region stays in the page so screen readers announce each message. `toasts` shows another
store (the design-system page's sample) instead of the app's. A toast slides in,
shows a bar of the time it has left (paused while the pointer or the focus is on it) and a × to close it;
on white: a round status mark and a time bar in the tone's color (success, error, info). No motion with reduced motion.
-->
<script lang="ts">
	import { flip } from 'svelte/animate';
	import { fade, fly } from 'svelte/transition';
	import { toast, type ToastStore } from '#lib/toast.svelte.ts';
	import Icon from './Icon.svelte';
	import { toastPositions, type ToastPosition } from './toast.ts';

	type Props = { position?: ToastPosition; toasts?: ToastStore };

	let { position = 'bottom-right', toasts = toast }: Props = $props();

	const reducedMotion =
		typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
	const motion = reducedMotion ? 0 : 1;
	const fromBelow = $derived(position.startsWith('bottom'));

	// Literal class names, so Tailwind builds them.
	const tones = { success: 'text-success', error: 'text-error', info: 'text-info' };
	const glyphs = { success: 'check', error: 'exclamation', info: 'letterI' } as const;
</script>

<div
	role="status"
	aria-live="polite"
	class={[
		'fixed z-50 flex w-[min(360px,calc(100vw-40px))] flex-col gap-2',
		toastPositions[position]
	]}
>
	{#each toasts.items as item (item.id)}
		<!-- Pausing is no action of its own: pointer and focus only hold the time (WCAG 2.2.1). -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			data-tone={item.tone}
			animate:flip={{ duration: 200 * motion }}
			in:fly={{ y: fromBelow ? 24 : -24, duration: 260 * motion }}
			out:fade={{ duration: 180 * motion }}
			onpointerenter={() => toasts.pause(item.id)}
			onpointerleave={() => toasts.resume(item.id)}
			onfocusin={() => toasts.pause(item.id)}
			onfocusout={() => toasts.resume(item.id)}
			class={[
				'relative flex items-start gap-3 overflow-hidden rounded-note bg-surface py-3.5 pr-2 pl-4 text-text shadow-toast forced-colors:border'
			]}
		>
			<span
				data-icon={item.tone}
				aria-hidden="true"
				class={[
					'mt-px grid size-5 shrink-0 place-items-center rounded-full bg-current',
					tones[item.tone]
				]}
			>
				<span class="text-surface"><Icon name={glyphs[item.tone]} size={14} /></span>
			</span>
			<p class="min-w-0 flex-1 wrap-break-word">{item.text}</p>
			<button
				type="button"
				aria-label="Close"
				title="Close"
				class="-mt-1.5 shrink-0 cursor-pointer rounded-control p-0.5 text-muted focus-ring hover:text-text"
				onclick={() => toasts.dismiss(item.id)}
			>
				<Icon name="close" size={16} />
			</button>
			<span
				aria-hidden="true"
				class={['absolute inset-x-0 bottom-0 h-1 motion-reduce:hidden', tones[item.tone]]}
			>
				<span class="absolute inset-0 bg-current opacity-20"></span>
				<span
					class={[
						'absolute inset-0 origin-left animate-toast-time bg-current',
						item.paused && '[animation-play-state:paused]'
					]}
					style:animation-duration="{item.duration}ms"
				></span>
			</span>
		</div>
	{/each}
</div>
