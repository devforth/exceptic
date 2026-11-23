import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import EditableTitleHarness from '../../test/EditableTitleHarness.svelte';

describe('EditableTitle', () => {
	it('turns into a field of the form around it, with no save of its own', async () => {
		const submitted: string[] = [];

		render(EditableTitleHarness, {
			title: 'backend',
			onsubmit: (name: string) => submitted.push(name)
		});
		await userEvent.click(screen.getByRole('button', { name: 'Rename' }));

		const field = screen.getByLabelText<HTMLInputElement>('Project name');

		expect(field.value).toBe('backend');
		expect(screen.queryByRole('button', { name: 'Cancel' })).toBeNull();
		await userEvent.clear(field);
		await userEvent.type(field, 'api{Enter}');

		expect(submitted).toEqual(['api']);
	});

	it('goes back to the title on Esc and drops what was typed', async () => {
		const submitted: string[] = [];

		render(EditableTitleHarness, {
			title: 'backend',
			onsubmit: (name: string) => submitted.push(name)
		});
		await userEvent.click(screen.getByRole('button', { name: 'Rename' }));
		await userEvent.type(screen.getByLabelText('Project name'), 'x{Escape}');

		expect(screen.getByRole('heading', { name: 'backend' })).toBeTruthy();
		await userEvent.click(screen.getByRole('button', { name: 'Submit' }));
		expect(submitted).toEqual(['backend']);
	});

	it('opens the field with the problem of a refused name', () => {
		render(EditableTitleHarness, {
			title: 'backend',
			error: 'This name is taken by another project.',
			onsubmit: () => {}
		});

		expect(screen.getByText('This name is taken by another project.')).toBeTruthy();
		expect(screen.getByLabelText('Project name').getAttribute('aria-invalid')).toBe('true');
	});
});
