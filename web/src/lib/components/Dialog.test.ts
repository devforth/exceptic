import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import Dialog from './Dialog.svelte';

const children = createRawSnippet(() => ({ render: () => '<p>Inside</p>' }));

function renderDialog(props: { dismissible?: boolean } = {}) {
	const onopen = vi.fn();
	const view = render(Dialog, { title: 'Sample', open: true, onopen, children, ...props });

	return { onopen, dialog: view.container.querySelector('dialog')! };
}

// Esc in a modal dialog, as a browser handles it: cancel, then close unless prevented.
function pressEscape(dialog: HTMLDialogElement) {
	if (dialog.dispatchEvent(new Event('cancel', { cancelable: true }))) dialog.close();
}

describe('Dialog', () => {
	it('opens as a modal and says so', () => {
		const { dialog, onopen } = renderDialog();

		expect(dialog.open).toBe(true);
		expect(onopen).toHaveBeenCalledOnce();
		expect(screen.getByText('Inside')).toBeTruthy();
	});

	it('closes with its button', async () => {
		const { dialog } = renderDialog();

		await userEvent.click(screen.getByRole('button', { name: 'Close' }));
		expect(dialog.open).toBe(false);
	});

	it('closes on Esc', () => {
		const { dialog } = renderDialog();

		pressEscape(dialog);
		expect(dialog.open).toBe(false);
	});

	it('closes on a click beside it (on the backdrop)', async () => {
		const { dialog } = renderDialog();

		await userEvent.click(screen.getByText('Inside'));
		expect(dialog.open).toBe(true);

		await userEvent.click(dialog);
		expect(dialog.open).toBe(false);
	});

	it('stays open while it is not dismissible', async () => {
		const { dialog } = renderDialog({ dismissible: false });

		await userEvent.click(screen.getByRole('button', { name: 'Close' }));
		pressEscape(dialog);
		await userEvent.click(dialog);

		expect(dialog.open).toBe(true);
	});
});
