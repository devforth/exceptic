import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { MenuItem } from './menu.ts';
import RowMenu from './RowMenu.svelte';

function renderMenu() {
	const rename = vi.fn();
	const remove = vi.fn();
	const items: MenuItem[] = [
		{ label: 'Rename', icon: 'edit', action: rename },
		{ label: 'Settings', icon: 'settings', href: '#settings', separatorBefore: true },
		{
			label: 'Delete',
			icon: 'delete',
			danger: true,
			separatorBefore: true,
			confirm: { title: 'Delete project', message: 'Delete backend?', confirmLabel: 'Delete it' },
			action: remove
		}
	];

	render(RowMenu, { label: 'Actions for backend', items });

	return { rename, remove, toggle: screen.getByRole('button', { name: 'Actions for backend' }) };
}

const menu = () => screen.queryByRole('menu');
const item = (name: string) => screen.getByRole('menuitem', { name });

describe('RowMenu', () => {
	it('opens a menu from its button, the first item focused', async () => {
		const { toggle } = renderMenu();

		expect(toggle.getAttribute('aria-haspopup')).toBe('menu');
		expect(toggle.getAttribute('aria-expanded')).toBe('false');
		expect(menu()).toBeNull();

		await userEvent.click(toggle);

		expect(toggle.getAttribute('aria-expanded')).toBe('true');
		expect(menu()?.getAttribute('aria-label')).toBe('Actions for backend');
		expect(screen.getAllByRole('menuitem').map((element) => element.textContent?.trim())).toEqual([
			'Rename',
			'Settings',
			'Delete'
		]);
		expect(document.activeElement).toBe(item('Rename'));
	});

	it('moves with the arrows, Home and End, wrapping around', async () => {
		const { toggle } = renderMenu();

		await userEvent.click(toggle);
		await userEvent.keyboard('{ArrowDown}');
		expect(document.activeElement).toBe(item('Settings'));
		await userEvent.keyboard('{ArrowDown}{ArrowDown}');
		expect(document.activeElement).toBe(item('Rename'));
		await userEvent.keyboard('{ArrowUp}');
		expect(document.activeElement).toBe(item('Delete'));
		await userEvent.keyboard('{Home}');
		expect(document.activeElement).toBe(item('Rename'));
		await userEvent.keyboard('{End}');
		expect(document.activeElement).toBe(item('Delete'));
	});

	it('closes on Esc back to its button, on Tab and on a click outside', async () => {
		const { toggle } = renderMenu();

		await userEvent.click(toggle);
		await userEvent.keyboard('{Escape}');
		expect(menu()).toBeNull();
		expect(document.activeElement).toBe(toggle);

		await userEvent.click(toggle);
		await userEvent.keyboard('{Tab}');
		expect(menu()).toBeNull();

		await userEvent.click(toggle);
		await userEvent.click(document.body);
		expect(menu()).toBeNull();
	});

	it('runs an action and closes', async () => {
		const { toggle, rename } = renderMenu();

		await userEvent.click(toggle);
		await userEvent.click(item('Rename'));

		expect(rename).toHaveBeenCalledOnce();
		expect(menu()).toBeNull();
	});

	it('asks before an action with a confirmation', async () => {
		const { toggle, remove } = renderMenu();

		await userEvent.click(toggle);
		await userEvent.click(item('Delete'));

		const dialog = screen.getByText('Delete backend?').closest('dialog')!;

		expect(remove).not.toHaveBeenCalled();
		expect(dialog.open).toBe(true);

		await userEvent.click(screen.getByRole('button', { name: 'Delete it' }));
		expect(remove).toHaveBeenCalledOnce();
		await waitFor(() => expect(dialog.open).toBe(false));
	});

	it('marks itself as an open popup, so Esc does not leave the page', async () => {
		const { toggle } = renderMenu();

		await userEvent.click(toggle);
		expect(menu()?.hasAttribute('data-popup')).toBe(true);
	});
});

describe('RowMenu with a disabled item', () => {
	it('lists it with its reason but does nothing on a click', async () => {
		const rename = vi.fn();

		render(RowMenu, {
			label: 'Actions for backend',
			items: [
				{ label: 'Rename it', icon: 'edit', disabled: 'Remove it from PROJECTS first.' },
				{ label: 'Name and color', icon: 'edit', action: rename }
			]
		});
		await userEvent.click(screen.getByRole('button', { name: 'Actions for backend' }));

		const disabled = screen.getByRole('menuitem', { name: /Rename it/ });

		expect(disabled.getAttribute('aria-disabled')).toBe('true');
		expect(screen.getByText('Remove it from PROJECTS first.').id).toBe(
			disabled.getAttribute('aria-describedby')
		);

		await userEvent.click(disabled);
		expect(menu()).not.toBeNull();

		await userEvent.keyboard('{ArrowDown}');
		expect(document.activeElement?.textContent?.trim()).toBe('Name and color');
	});
});
