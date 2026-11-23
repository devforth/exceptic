// The state of a form that sends one request: busy while it runs, the server's refusal
// split into the problems of each field (shown by TextField), the record that already does
// what was asked (duplicate) and a message for the rest. A form that edits a saved record
// gives its values: it sends nothing while they are the saved ones (SubmitButton shows it).
import { ApiError, errorMessage } from '#lib/api/client.ts';

const serialized = (values: unknown) => JSON.stringify($state.snapshot(values));

export class FormState {
	#values: (() => unknown) | undefined;
	/** The saved values, serialized: what the form started with or last saved. */
	#saved = $state.raw('');

	busy = $state(false);
	/** A refusal that names no field nor a duplicate, or a failure. */
	error = $state('');
	/** The problems of the fields, by input name. */
	fields: Record<string, string> = $state({});
	/** The record that already does it: the form links to it instead of a message. */
	duplicate: { id: string; name: string } | undefined = $state();

	/**
	 * values: what the form would send (trimmed as the server trims, an empty secret field
	 * left out); its first values are the saved ones. None for a form that creates something.
	 */
	constructor(values?: () => unknown) {
		this.#values = values;
		if (values) this.#saved = serialized(values());
	}

	/** Whether the values differ from the saved ones; always true without values. */
	get changed(): boolean {
		return !this.#values || serialized(this.#values()) !== this.#saved;
	}

	/** The values shown now are the saved ones (a dialog opened for a record, a reload), no problems shown. */
	reset() {
		if (this.#values) this.#saved = serialized(this.#values());
		this.error = '';
		this.fields = {};
		this.duplicate = undefined;
	}

	/**
	 * Runs the request; true when it succeeded, and the values then shown are the saved ones.
	 * A second submit while busy, or one with nothing changed, is ignored.
	 */
	async submit(request: () => Promise<unknown>): Promise<boolean> {
		if (this.busy || !this.changed) return false;

		this.busy = true;
		this.error = '';
		this.fields = {};
		this.duplicate = undefined;

		try {
			await request();
			if (this.#values) this.#saved = serialized(this.#values());

			return true;
		} catch (failure) {
			if (failure instanceof ApiError && failure.duplicate) {
				this.duplicate = failure.duplicate;
			} else if (
				failure instanceof ApiError &&
				failure.fields &&
				Object.keys(failure.fields).length
			) {
				this.fields = failure.fields;
			} else {
				this.error = errorMessage(failure);
			}

			return false;
		} finally {
			this.busy = false;
		}
	}
}
