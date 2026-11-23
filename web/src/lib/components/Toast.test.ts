import { fireEvent, render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ToastStore, toast } from '#lib/toast.svelte.ts';
import Toast from './Toast.svelte';

beforeEach(() => {
	toast.clear();
});

afterEach(() => {
	vi.useRealTimers();
	toast.clear();
});

const region = () => screen.getByRole('status');

describe('Toast', () => {
	it('stacks messages instead of replacing them, each in its tone', async () => {
		render(Toast);
		toast.show('Saved.');
		toast.show('Could not copy.', 'error');

		await vi.waitFor(() => within(region()).getByText('Could not copy.'));
		expect(within(region()).getByText('Saved.')).toBeTruthy();
		expect(
			within(region())
				.getByText('Could not copy.')
				.closest('[data-tone]')
				?.getAttribute('data-tone')
		).toBe('error');
		expect(
			within(region()).getByText('Saved.').closest('[data-tone]')?.getAttribute('data-tone')
		).toBe('success');
	});

	it('closes one message with its ×', async () => {
		render(Toast);
		toast.show('Saved.');
		toast.show('Connection added.');

		const saved = await vi.waitFor(() => within(region()).getByText('Saved.'));
		await userEvent.click(
			within(saved.closest('[data-tone]') as HTMLElement).getByRole('button', { name: 'Close' })
		);

		await vi.waitFor(() => expect(within(region()).queryByText('Saved.')).toBeNull());
		expect(within(region()).getByText('Connection added.')).toBeTruthy();
	});

	it('goes after its time, and waits while the pointer rests on it', async () => {
		vi.useFakeTimers();
		render(Toast);
		toast.show('Saved.', 'info', 1000);

		const item = (await vi.waitFor(() => within(region()).getByText('Saved.'))).closest(
			'[data-tone]'
		) as HTMLElement;

		await fireEvent.pointerEnter(item);
		vi.advanceTimersByTime(5000);
		expect(toast.items).toHaveLength(1);

		await fireEvent.pointerLeave(item);
		vi.advanceTimersByTime(1000);
		expect(toast.items).toHaveLength(0);
	});

	it('keeps the newest five', () => {
		for (const n of [1, 2, 3, 4, 5, 6]) toast.show(`Message ${n}.`);

		expect(toast.items.map((item) => item.text)).toEqual([
			'Message 2.',
			'Message 3.',
			'Message 4.',
			'Message 5.',
			'Message 6.'
		]);
	});

	it('marks each tone with its status icon', async () => {
		render(Toast);
		toast.show('Sent again.', 'info');

		const item = (await vi.waitFor(() => within(region()).getByText('Sent again.'))).closest(
			'[data-tone]'
		) as HTMLElement;

		expect(item.getAttribute('data-tone')).toBe('info');
		expect(item.querySelector('[data-icon="info"]')).toBeTruthy();
	});

	it('sits in the corner it is given', () => {
		render(Toast, { position: 'top-left' });

		expect(region().className).toContain('top-5');
		expect(region().className).toContain('left-5');
	});

	it("shows the toasts of the store it is given, not the app's", async () => {
		const sample = new ToastStore();

		render(Toast, { toasts: sample });
		toast.show('From the app.');
		sample.show('From the sample.', 'info');

		await vi.waitFor(() => within(region()).getByText('From the sample.'));
		expect(within(region()).queryByText('From the app.')).toBeNull();
		sample.clear();
	});
});
