// The state of a form that sends one request: busy while it runs, the server's refusal
// split into the problems of each field (shown by TextField), the record that already does
// what was asked (duplicate) and a message for the rest.
import { ApiError } from '#lib/api/client.ts';

export class FormState {
	busy = $state(false);
	/** A refusal that names no field nor a duplicate, or a failure. */
	error = $state('');
	/** The problems of the fields, by input name. */
	fields: Record<string, string> = $state({});
	/** The record that already does it: the form links to it instead of a message. */
	duplicate: { id: string; name: string } | undefined = $state();

	/** Runs the request; true when it succeeded. A second submit while busy is ignored. */
	async submit(request: () => Promise<unknown>): Promise<boolean> {
		if (this.busy) return false;

		this.busy = true;
		this.error = '';
		this.fields = {};
		this.duplicate = undefined;

		try {
			await request();

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
				this.error = failure instanceof ApiError ? failure.message : 'Something went wrong.';
			}

			return false;
		} finally {
			this.busy = false;
		}
	}
}
