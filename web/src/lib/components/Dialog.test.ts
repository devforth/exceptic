import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import Dialog from './Dialog.svelte';

const children = createRawSnippet(() => ({ render: () => '<p>Inside</p>' }));

const actions = createRawSnippet(() => ({
	render: () => '<button type="button">Cancel</button>'
}));

function renderDialog(props: { dismissible?: boolean; size?: 'wide' | 'narrow' } = {}) {
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

	it('is named by its title', () => {
		const { dialog } = renderDialog();

		expect(screen.getByRole('heading', { name: 'Sample' })).toBeTruthy();
		expect(dialog.getAttribute('aria-labelledby')).toBe(
			screen.getByRole('heading', { name: 'Sample' }).id
		);
	});

	it('narrow: title, content and its own buttons, no Close; Esc still closes it', () => {
		const onopen = vi.fn();
		const view = render(Dialog, {
			title: 'Sample',
			open: true,
			size: 'narrow',
			onopen,
			children,
			actions
		});
		const dialog = view.container.querySelector('dialog')!;

		expect(screen.getByRole('heading', { name: 'Sample' })).toBeTruthy();
		expect(screen.getByRole('button', { name: 'Cancel' })).toBeTruthy();
		expect(screen.queryByRole('button', { name: 'Close' })).toBeNull();

		pressEscape(dialog);
		expect(dialog.open).toBe(false);
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

		// A browser may close a modal on its own (a close watcher): it opens again.
		dialog.close();

		expect(dialog.open).toBe(true);
	});
});
