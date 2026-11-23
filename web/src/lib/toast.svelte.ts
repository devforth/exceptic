// The toasts of the app: Toast.svelte shows them, anything may call show. Each stays its time
// (paused while the pointer rests on it) or until closed; the newest few are kept.
const shownFor = 5000;
const kept = 5;

export type ToastTone = 'success' | 'error' | 'info';

export type ToastItem = {
	id: number;
	text: string;
	tone: ToastTone;
	/** Its whole time on screen, in ms: the progress bar runs over it. */
	duration: number;
	paused: boolean;
};

let items: ToastItem[] = $state([]);
let lastId = 0;

// Per toast: its timer, when it last started and the time it still has (not shown: no state).
// eslint-disable-next-line svelte/prefer-svelte-reactivity
const clocks = new Map<
	number,
	{ timer?: ReturnType<typeof setTimeout>; started: number; left: number }
>();

function run(id: number) {
	const clock = clocks.get(id);

	if (!clock) return;

	clock.started = Date.now();
	clock.timer = setTimeout(() => toast.dismiss(id), clock.left);
}

export const toast = {
	get items(): readonly ToastItem[] {
		return items;
	},
	/** A done action ("Saved.") by default; a failure with tone error, a note with info. */
	show(text: string, tone: ToastTone = 'success', duration = shownFor) {
		const id = ++lastId;

		items = [...items, { id, text, tone, duration, paused: false }];
		clocks.set(id, { started: Date.now(), left: duration });
		run(id);

		for (const old of items.slice(0, Math.max(0, items.length - kept))) toast.dismiss(old.id);
	},
	dismiss(id: number) {
		clearTimeout(clocks.get(id)?.timer);
		clocks.delete(id);
		items = items.filter((item) => item.id !== id);
	},
	pause(id: number) {
		const clock = clocks.get(id);
		const item = items.find((entry) => entry.id === id);

		if (!clock || !item || item.paused) return;

		clearTimeout(clock.timer);
		clock.left = Math.max(0, clock.left - (Date.now() - clock.started));
		item.paused = true;
	},
	resume(id: number) {
		const item = items.find((entry) => entry.id === id);

		if (!item?.paused) return;

		item.paused = false;
		run(id);
	},
	clear() {
		for (const item of items) toast.dismiss(item.id);
	}
};
