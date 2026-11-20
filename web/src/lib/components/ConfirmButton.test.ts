import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api/client.ts';
import ConfirmButton from './ConfirmButton.svelte';

const children = createRawSnippet(() => ({ render: () => '<span>Delete</span>' }));

const confirm = {
	title: 'Delete project',
	message: 'Delete the project backend?',
	confirmLabel: 'Delete it'
};

function renderButton(action: () => unknown) {
	const view = render(ConfirmButton, { confirm, action, children });

	return { dialog: view.container.querySelector('dialog')! };
}

describe('ConfirmButton and ConfirmDialog', () => {
	it('asks first, with Cancel focused', async () => {
		const action = vi.fn();
		const { dialog } = renderButton(action);

		expect(dialog.open).toBe(false);

		await userEvent.click(screen.getByRole('button', { name: 'Delete' }));

		expect(dialog.open).toBe(true);
		expect(screen.getByText('Delete the project backend?')).toBeTruthy();
		expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Cancel' }));
		expect(action).not.toHaveBeenCalled();
	});

	it('does nothing on Cancel or Esc', async () => {
		const action = vi.fn();
		const { dialog } = renderButton(action);

		await userEvent.click(screen.getByRole('button', { name: 'Delete' }));
		await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));
		expect(dialog.open).toBe(false);

		await userEvent.click(screen.getByRole('button', { name: 'Delete' }));
		dialog.dispatchEvent(new Event('cancel', { cancelable: true }));
		dialog.close();
		expect(dialog.open).toBe(false);

		expect(action).not.toHaveBeenCalled();
	});

	it('runs the action once confirmed, busy and not dismissible meanwhile, then closes', async () => {
		let finish = () => {};
		const action = vi.fn(() => new Promise<void>((resolve) => (finish = resolve)));
		const { dialog } = renderButton(action);

		await userEvent.click(screen.getByRole('button', { name: 'Delete' }));
		await userEvent.click(screen.getByRole('button', { name: 'Delete it' }));

		expect(action).toHaveBeenCalledOnce();
		expect(screen.getByRole('button', { name: 'Delete it' }).getAttribute('aria-busy')).toBe(
			'true'
		);

		// A second click while it runs does not run it again; Esc does not close it.
		await userEvent.click(screen.getByRole('button', { name: 'Delete it' }));
		expect(action).toHaveBeenCalledOnce();
		expect(dialog.dispatchEvent(new Event('cancel', { cancelable: true }))).toBe(false);

		finish();
		await waitFor(() => expect(dialog.open).toBe(false));
	});

	it('shows why the action failed and stays open', async () => {
		const { dialog } = renderButton(() => Promise.reject(new ApiError(400, 'project is in use')));

		await userEvent.click(screen.getByRole('button', { name: 'Delete' }));
		await userEvent.click(screen.getByRole('button', { name: 'Delete it' }));

		await waitFor(() => expect(screen.getByText('project is in use')).toBeTruthy());
		expect(dialog.open).toBe(true);
	});
});
