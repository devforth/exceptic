import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Breadcrumbs from './Breadcrumbs.svelte';

// Followed links: the click is stopped, jsdom does not navigate.
const followed = vi.fn();

function renderCrumbs() {
	render(Breadcrumbs, {
		items: [
			{ label: 'Projects', href: '/projects' },
			{ label: 'backend', href: '/projects/1' },
			{ label: 'Installation', href: '/projects/1/setup' }
		]
	});

	for (const link of screen.getAllByRole('link')) {
		link.addEventListener('click', (event) => {
			event.preventDefault();
			followed(link.getAttribute('href'));
		});
	}
}

function pressEscape(target: Element = document.body) {
	target.dispatchEvent(
		new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
	);
	target.dispatchEvent(new KeyboardEvent('keyup', { key: 'Escape', bubbles: true }));
}

afterEach(() => followed.mockClear());

describe('Breadcrumbs', () => {
	it('shows the path, the last item the current page', () => {
		renderCrumbs();

		expect(screen.getByRole('navigation', { name: 'Breadcrumb' })).toBeTruthy();
		expect(screen.getByRole('link', { name: 'Installation' }).getAttribute('aria-current')).toBe(
			'page'
		);
		expect(screen.getByRole('link', { name: 'backend' }).title).toBe('Back (Esc)');
	});

	it('goes one level up on Esc, on keyup', () => {
		renderCrumbs();

		document.body.dispatchEvent(
			new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
		);
		expect(followed).not.toHaveBeenCalled();

		document.body.dispatchEvent(new KeyboardEvent('keyup', { key: 'Escape', bubbles: true }));
		expect(followed).toHaveBeenCalledExactlyOnceWith('/projects/1');
	});

	it('leaves Esc to a field being typed in', () => {
		renderCrumbs();

		const field = document.body.appendChild(document.createElement('input'));
		pressEscape(field);

		expect(followed).not.toHaveBeenCalled();
		field.remove();
	});

	it('leaves Esc to an open dialog, menu or dropdown', () => {
		renderCrumbs();

		for (const markup of [
			'<dialog open></dialog>',
			'<details open data-popup><summary>Projects</summary></details>',
			'<div role="menu" data-popup></div>'
		]) {
			const popup = document.createElement('div');
			popup.innerHTML = markup;
			document.body.append(popup);
			pressEscape();
			popup.remove();
		}

		expect(followed).not.toHaveBeenCalled();

		// A closed dropdown does not count.
		const closed = document.createElement('div');
		closed.innerHTML = '<details data-popup><summary>Projects</summary></details>';
		document.body.append(closed);
		pressEscape();
		expect(followed).toHaveBeenCalledOnce();
		closed.remove();
	});

	it('leaves an Esc another element handled', () => {
		renderCrumbs();

		const handled = new KeyboardEvent('keydown', {
			key: 'Escape',
			bubbles: true,
			cancelable: true
		});
		handled.preventDefault();
		document.body.dispatchEvent(handled);
		document.body.dispatchEvent(new KeyboardEvent('keyup', { key: 'Escape', bubbles: true }));

		expect(followed).not.toHaveBeenCalled();
	});
});
