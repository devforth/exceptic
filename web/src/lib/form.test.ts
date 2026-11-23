import { describe, expect, it, vi } from 'vitest';
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

	it('sends nothing until a value differs from the saved ones', async () => {
		const values = { name: 'backend', levels: ['fatal', 'error'] };
		const form = new FormState(() => values);
		const request = vi.fn(() => Promise.resolve());

		expect(form.changed).toBe(false);
		expect(await form.submit(request)).toBe(false);
		expect(request).not.toHaveBeenCalled();

		values.levels = ['fatal'];
		expect(form.changed).toBe(true);
		values.levels = ['fatal', 'error'];
		expect(form.changed).toBe(false);

		values.name = 'api';
		expect(await form.submit(request)).toBe(true);
		expect(request).toHaveBeenCalledTimes(1);
		// Saved: these are the values to compare with now.
		expect(form.changed).toBe(false);
		values.name = 'backend';
		expect(form.changed).toBe(true);
	});

	it('stays changed after a refused save, and reset takes the values shown as saved', async () => {
		const values = { name: 'backend' };
		const form = new FormState(() => values);

		values.name = 'api';
		await form.submit(() =>
			Promise.reject(new ApiError(400, 'refused', { fields: { name: 'taken' } }))
		);
		expect(form.changed).toBe(true);

		form.reset();
		expect(form.changed).toBe(false);
		expect(form.fields).toEqual({});
	});

	it('counts every form without its values as changed', async () => {
		const form = new FormState();

		expect(form.changed).toBe(true);
		expect(await form.submit(() => Promise.resolve())).toBe(true);
		expect(form.changed).toBe(true);
	});
});
