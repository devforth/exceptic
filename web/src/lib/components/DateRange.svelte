<!--
@component
A range of UTC days, both included, in one field: "20 Nov – 23 Nov", × clears it. The field
opens a month calendar (Monday first, up to today): the first day chosen starts the range, the
second ends it (a reversed range is swapped) and closes the calendar. Keyboard in the calendar:
the arrows move by a day or a week, Page Up and Page Down by a month (with Shift a year), Home
and End to the ends of the week, Enter or Space choose, Esc (also on the field or ×) closes it
back on the field. The calendar is fixed under the field, inside the viewport (popup.ts); a click
outside, scrolling or resizing close it.
-->
<script lang="ts">
	import { tick } from 'svelte';
	import { today } from '#lib/filters.ts';
	import {
		addDays,
		addMonths,
		dayName,
		monthOf,
		monthTitle,
		monthWeeks,
		rangeText,
		weekdayOf,
		weekdays
	} from './dates.ts';
	import { controlClasses } from './field.ts';
	import Icon from './Icon.svelte';
	import { closeWhenLeft, openControlClasses, popupPlace, type PopupPlace } from './popup.ts';
	import { popupClasses } from './surface.ts';

	type Props = { from: string; to: string; onchange: (from: string, to: string) => void };

	let { from, to, onchange }: Props = $props();

	const id = $props.id();
	const last = today();

	let open = $state(false);
	// The month shown, the day with the focus, and the first day of a range being chosen.
	let month = $state(monthOf(last));
	let focused = $state(last);
	let start: string | undefined = $state();
	let pointed: string | undefined = $state();

	let box: HTMLElement;
	let field: HTMLButtonElement;
	let grid: HTMLElement | undefined = $state();
	let calendar: HTMLElement | undefined = $state();
	let place: PopupPlace = $state({ top: 0, left: 0, minWidth: 0 });

	const text = $derived(rangeText(from, to, last));

	// The range drawn: the one being chosen up to the day pointed at, or the chosen one.
	const shown = $derived.by(() => {
		if (start !== undefined) {
			const end = pointed ?? start;

			return start < end ? { first: start, end } : { first: end, end: start };
		}

		return { first: from || to, end: to || from };
	});

	const later = (day: string) => (day > last ? last : day);

	async function focusDay(day: string) {
		focused = later(day);
		month = monthOf(focused);
		await tick();
		grid?.querySelector<HTMLElement>(`[data-day="${focused}"]`)?.focus();
	}

	async function show() {
		open = true;
		start = undefined;
		pointed = undefined;
		await tick();

		place = popupPlace(field.getBoundingClientRect(), {
			width: calendar?.offsetWidth ?? 0,
			height: calendar?.offsetHeight ?? 0
		});
		void focusDay(from || to || last);
	}

	// Another month by its buttons: the focus day goes along, so the grid keeps a Tab stop.
	function turn(count: number) {
		month = addMonths(month, count);
		focused = later(addMonths(focused, count));
	}

	function close(focusField: boolean) {
		open = false;

		if (focusField) field.focus();
	}

	function pick(day: string) {
		if (start === undefined) {
			start = day;
			focused = day;

			return;
		}

		const [first, end] = start < day ? [start, day] : [day, start];

		start = undefined;
		close(true);
		onchange(first, end);
	}

	function clear() {
		field.focus();
		onchange('', '');
	}

	function gridKeydown(event: KeyboardEvent) {
		const moves: Record<string, () => string> = {
			ArrowLeft: () => addDays(focused, -1),
			ArrowRight: () => addDays(focused, 1),
			ArrowUp: () => addDays(focused, -7),
			ArrowDown: () => addDays(focused, 7),
			PageUp: () => addMonths(focused, event.shiftKey ? -12 : -1),
			PageDown: () => addMonths(focused, event.shiftKey ? 12 : 1),
			Home: () => addDays(focused, -weekdayOf(focused)),
			End: () => addDays(focused, 6 - weekdayOf(focused))
		};
		const move = moves[event.key];

		if (!move) return;

		event.preventDefault();
		pointed = start === undefined ? undefined : later(move());
		void focusDay(move());
	}

	function keydown(event: KeyboardEvent) {
		if (open && event.key === 'Escape') {
			// Handled: Esc must not also leave the page (Breadcrumbs).
			event.preventDefault();
			close(true);
		}
	}

	// Tab out of the calendar closes it.
	function focusout(event: FocusEvent) {
		if (open && !box.contains(event.relatedTarget as Node | null)) close(false);
	}

	$effect(() => {
		if (!open) return;

		return closeWhenLeft(() => [box], close);
	});

	// Where a day sits in the chosen range: the range is one band, its first day rounded on the
	// left, its last on the right, the days between square (a range of one day is a circle).
	function bandOf(day: string): 'start' | 'between' | 'end' | undefined {
		if (!shown.first || shown.first === shown.end) return undefined;
		if (day === shown.first) return 'start';
		if (day === shown.end) return 'end';

		return day > shown.first && day < shown.end ? 'between' : undefined;
	}

	// Literal class names, so Tailwind builds them.
	const shapes = {
		start: 'rounded-l-full bg-accent font-semibold text-accent-text',
		between: 'bg-selected',
		end: 'rounded-r-full bg-accent font-semibold text-accent-text'
	};

	function dayClasses(day: string) {
		const band = bandOf(day);
		const single = shown.first !== undefined && day === shown.first && day === shown.end;

		return [
			'h-9 w-full min-w-9 cursor-pointer text-sm tabular-nums focus-ring [--focus-offset:-2px] disabled:cursor-not-allowed disabled:opacity-30',
			band
				? shapes[band]
				: single
					? 'rounded-full bg-accent font-semibold text-accent-text'
					: [
							'rounded-full enabled:hover:bg-hover',
							day === last
								? 'font-bold text-accent ring-2 ring-accent ring-inset'
								: day.slice(0, 7) !== month.slice(0, 7) && 'text-muted'
						],
			band === 'between' && day === last && 'font-bold text-accent'
		];
	}

	const navClasses =
		'inline-flex size-8 cursor-pointer items-center justify-center rounded-control text-muted focus-ring hover:bg-hover hover:text-accent disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent';
</script>

<!-- The focus leaving the field and its calendar closes it; Esc anywhere in them too. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<span bind:this={box} class="relative inline-flex" onfocusout={focusout} onkeydown={keydown}>
	<button
		bind:this={field}
		type="button"
		aria-haspopup="dialog"
		aria-expanded={open}
		aria-controls={open ? `${id}-calendar` : undefined}
		title="Days in UTC"
		class={[
			controlClasses,
			'w-[220px] cursor-pointer truncate pr-7 text-left transition-[border-color,box-shadow]',
			open && openControlClasses
		]}
		onclick={() => (open ? close(false) : show())}
	>
		{#if text}<span class="sr-only">Date range:</span> {text}{:else}<span class="text-muted">
				Date range
			</span>{/if}
	</button>
	{#if text}
		<button
			type="button"
			title="Clear dates"
			aria-label="Clear dates"
			class="absolute top-1/2 right-1 -translate-y-1/2 cursor-pointer rounded-control px-1 text-h2 leading-none text-muted focus-ring hover:text-text"
			onclick={clear}
		>
			×
		</button>
	{/if}

	{#if open}
		<div
			bind:this={calendar}
			id="{id}-calendar"
			role="dialog"
			aria-label="Choose dates"
			tabindex="-1"
			data-popup
			class={[popupClasses, 'fixed z-40 text-text']}
			style:top="{place.top}px"
			style:left="{place.left}px"
		>
			<div class="flex items-center justify-between gap-2 px-1 pt-1 pb-2">
				<button
					type="button"
					class={navClasses}
					aria-label="Previous month"
					onclick={() => turn(-1)}
				>
					<span class="flex rotate-180"><Icon name="chevronRight" size={16} /></span>
				</button>
				<span id="{id}-month" class="font-semibold" aria-live="polite">{monthTitle(month)}</span>
				<button
					type="button"
					class={navClasses}
					aria-label="Next month"
					disabled={addMonths(month, 1) > last}
					onclick={() => turn(1)}
				>
					<Icon name="chevronRight" size={16} />
				</button>
			</div>
			<table
				bind:this={grid}
				role="grid"
				aria-labelledby="{id}-month"
				class="border-collapse"
				onkeydown={gridKeydown}
				onpointerleave={() => (pointed = undefined)}
			>
				<thead>
					<tr>
						{#each weekdays as weekday (weekday)}
							<th scope="col" class="h-7 text-xs font-normal text-muted">
								<abbr title={weekday} class="no-underline">{weekday.slice(0, 2)}</abbr>
							</th>
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each monthWeeks(month) as week (week[0])}
						<tr>
							{#each week as day (day)}
								{@const band = bandOf(day)}
								<td
									role="gridcell"
									data-band={band}
									class="p-0 text-center"
									aria-selected={Boolean(shown.first) && day >= shown.first && day <= shown.end}
								>
									<button
										type="button"
										data-day={day}
										tabindex={day === focused ? 0 : -1}
										disabled={day > last}
										aria-label={dayName(day)}
										aria-current={day === last ? 'date' : undefined}
										class={dayClasses(day)}
										onclick={() => pick(day)}
										onpointerenter={() => (pointed = start === undefined ? undefined : day)}
										onfocus={() => (focused = day)}
									>
										{Number(day.slice(8, 10))}
									</button>
								</td>
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
			<p class="sr-only" aria-live="polite">
				{start === undefined ? 'Choose the first day.' : 'Choose the last day.'}
			</p>
		</div>
	{/if}
</span>
