import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { roleSummaries } from '#lib/api/catalog.gen.ts';
import RoleChoices from './RoleChoices.svelte';

describe('RoleChoices', () => {
	it('offers the roles an administrator gives, with what each may do', async () => {
		render(RoleChoices, { value: 'user', error: 'Choose a role.' });

		const admin = screen.getByRole<HTMLInputElement>('radio', { name: /admin/ });

		expect(screen.getAllByRole('radio').map((radio) => (radio as HTMLInputElement).value)).toEqual([
			'admin',
			'user'
		]);
		expect(screen.queryByRole('radio', { name: /superadmin/ })).toBeNull();
		expect(screen.getByText(roleSummaries.admin)).toBeTruthy();
		expect(screen.getByRole<HTMLInputElement>('radio', { name: /^user/ }).checked).toBe(true);
		expect(screen.getByText('Choose a role.')).toBeTruthy();

		await userEvent.click(admin);

		expect(admin.checked).toBe(true);
	});

	it('offers only the roles the viewer may give: an admin gives only user', () => {
		render(RoleChoices, { value: 'user', roles: ['user'] });

		expect(screen.getAllByRole('radio').map((radio) => (radio as HTMLInputElement).value)).toEqual([
			'user'
		]);
	});
});
