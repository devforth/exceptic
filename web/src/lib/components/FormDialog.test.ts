import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api/client.ts';
import FormDialogHarness from '../../test/FormDialogHarness.svelte';

describe('FormDialog', () => {
	it('keeps a refused value with its problem, closes once saved', async () => {
		const user = userEvent.setup();
		const save = vi
			.fn()
			.mockRejectedValueOnce(
				new ApiError(400, 'refused', { fields: { name: 'The name is taken.' } })
			)
			.mockResolvedValueOnce(undefined);

		render(FormDialogHarness, { save });
		const dialog = document.querySelector('dialog')!;

		await user.type(screen.getByLabelText(/^Name/), '{Enter}');
		await waitFor(() => expect(screen.getByText('The name is taken.')).toBeTruthy());
		expect(dialog.open).toBe(true);

		await user.click(screen.getByRole('button', { name: 'Save' }));
		await waitFor(() => expect(dialog.open).toBe(false));
		expect(save).toHaveBeenCalledTimes(2);
	});

	it('shows a failure that names no field', async () => {
		const save = vi.fn().mockRejectedValueOnce(new ApiError(500, 'internal error'));

		render(FormDialogHarness, { save });
		await userEvent.click(screen.getByRole('button', { name: 'Save' }));

		await waitFor(() => expect(screen.getByText('internal error')).toBeTruthy());
		expect(document.querySelector('dialog')!.open).toBe(true);
	});
});
