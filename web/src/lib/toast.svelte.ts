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

type Clock = { timer?: ReturnType<typeof setTimeout>; started: number; left: number };

/** A stack of toasts; the app has one (`toast`), the design-system page its own sample. */
export class ToastStore {
	#items: ToastItem[] = $state([]);
	#lastId = 0;
	// Per toast: its timer, when it last started and the time it still has (not shown: no state).
	// eslint-disable-next-line svelte/prefer-svelte-reactivity
	#clocks = new Map<number, Clock>();

	get items(): readonly ToastItem[] {
		return this.#items;
	}

	/** A done action ("Saved.") by default; a failure with tone error, a note with info. */
	show(text: string, tone: ToastTone = 'success', duration = shownFor) {
		const id = ++this.#lastId;

		this.#items = [...this.#items, { id, text, tone, duration, paused: false }];
		this.#clocks.set(id, { started: Date.now(), left: duration });
		this.#run(id);

		for (const old of this.#items.slice(0, Math.max(0, this.#items.length - kept))) {
			this.dismiss(old.id);
		}
	}

	dismiss(id: number) {
		clearTimeout(this.#clocks.get(id)?.timer);
		this.#clocks.delete(id);
		this.#items = this.#items.filter((item) => item.id !== id);
	}

	pause(id: number) {
		const clock = this.#clocks.get(id);
		const item = this.#items.find((entry) => entry.id === id);

		if (!clock || !item || item.paused) return;

		clearTimeout(clock.timer);
		clock.left = Math.max(0, clock.left - (Date.now() - clock.started));
		item.paused = true;
	}

	resume(id: number) {
		const item = this.#items.find((entry) => entry.id === id);

		if (!item?.paused) return;

		item.paused = false;
		this.#run(id);
	}

	clear() {
		for (const item of this.#items) this.dismiss(item.id);
	}

	#run(id: number) {
		const clock = this.#clocks.get(id);

		if (!clock) return;

		clock.started = Date.now();
		clock.timer = setTimeout(() => this.dismiss(id), clock.left);
	}
}

export const toast = new ToastStore();
