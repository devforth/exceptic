import { describe, expect, it } from 'vitest';
import { ApiError } from '#lib/api/client.ts';
import { FormState } from './form.svelte.ts';

describe('FormState', () => {
	it('puts a refusal on its fields, other failures in the message', async () => {
		const form = new FormState();

		expect(
			await form.submit(() =>
				Promise.reject(new ApiError(400, 'name: taken', { fields: { name: 'taken' } }))
			)
		).toBe(false);
		expect(form.fields).toEqual({ name: 'taken' });
		expect(form.error).toBe('');

		await form.submit(() => Promise.reject(new ApiError(404, 'not found')));
		expect(form.fields).toEqual({});
		expect(form.error).toBe('not found');

		await form.submit(() =>
			Promise.reject(
				new ApiError(400, 'already sends there', { duplicate: { id: 'abcd1234', name: 'Ops' } })
			)
		);
		expect(form.duplicate).toEqual({ id: 'abcd1234', name: 'Ops' });
		expect(form.error).toBe('');

		expect(await form.submit(() => Promise.resolve())).toBe(true);
		expect(form.duplicate).toBeUndefined();
		expect(form.error).toBe('');
		expect(form.busy).toBe(false);
	});

	it('ignores a second submit while busy', async () => {
		const form = new FormState();
		let runs = 0;
		let finish = () => {};
		const first = form.submit(() => new Promise<void>((resolve) => ((finish = resolve), runs++)));

		expect(await form.submit(() => Promise.resolve(runs++))).toBe(false);
		finish();
		await first;
		expect(runs).toBe(1);
	});
});
