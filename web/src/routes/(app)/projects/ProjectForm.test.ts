import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import ProjectForm from './ProjectForm.svelte';

vi.mock('$app/paths', () => ({
	resolve: (route: string, params?: { id?: number }) =>
		route.replace('/(app)', '').replace('[id=projectId]', String(params?.id ?? ''))
}));

const storage = {
	used_bytes: 0,
	used: '0 B',
	cleanup_stop_percent: 80,
	projects: [],
	other_bytes: 0,
	other: '0 B'
};

function renderForm(save = vi.fn(() => Promise.resolve())) {
	render(ProjectForm, {
		id: 1,
		name: 'backend',
		initial: { color: '#2563eb', logLevels: ['fatal', 'error'], diskShare: 100 },
		storage,
		save
	});

	return save;
}

const saveButton = () => screen.getByRole('button', { name: 'Save' });
const inactive = () => saveButton().getAttribute('aria-disabled') === 'true';

describe('ProjectForm', () => {
	it('sends nothing until a value changes, and again once it is saved or undone', async () => {
		const user = userEvent.setup();
		const save = renderForm();

		expect(inactive()).toBe(true);
		expect(saveButton().title).toBe('Nothing changed');
		await user.click(saveButton());
		expect(save).not.toHaveBeenCalled();

		await user.click(screen.getByRole('checkbox', { name: 'warn' }));
		expect(inactive()).toBe(false);
		await user.click(screen.getByRole('checkbox', { name: 'warn' }));
		expect(inactive()).toBe(true);

		await user.click(screen.getByRole('checkbox', { name: 'warn' }));
		await user.click(saveButton());
		await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
		expect(save).toHaveBeenCalledWith(
			{ color: '#2563eb', logLevels: ['fatal', 'error', 'warn'], diskShare: 100 },
			'backend'
		);
		await waitFor(() => expect(inactive()).toBe(true));
		// The focus stays on the button it pressed.
		expect(document.activeElement).toBe(saveButton());
	});

	it('shows the disk share read-only with why to a viewer who may not change it', () => {
		const reason = 'Only someone with access to all projects can change the disk share.';

		render(ProjectForm, {
			id: 1,
			name: 'backend',
			initial: { color: '#2563eb', logLevels: ['fatal', 'error'], diskShare: 100 },
			storage,
			diskShareLocked: reason,
			save: vi.fn(() => Promise.resolve())
		});

		const slider = screen.getByRole<HTMLInputElement>('slider');

		expect(slider.disabled).toBe(true);
		expect(slider.getAttribute('aria-describedby')).toContain('locked');
		expect(screen.getByText(reason)).toBeTruthy();
	});
});
