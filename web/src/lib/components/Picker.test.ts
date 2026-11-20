import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import Picker from './Picker.svelte';

const option = createRawSnippet((value: () => string) => ({
	render: () => `<span>${value()}</span>`
}));

function renderPicker(selected: string[] = [], options = ['backend', 'frontend', 'worker']) {
	const onchange = vi.fn();
	const view = render(Picker, {
		label: 'Projects',
		all: 'All projects',
		options,
		selected,
		onchange,
		option
	});

	return {
		onchange,
		details: view.container.querySelector('details')!,
		summary: view.container.querySelector('summary')!
	};
}

describe('Picker', () => {
	it('says what is chosen on its button', () => {
		renderPicker(['frontend', 'worker']);

		expect(screen.getByText('Projects: 2')).toBeTruthy();
	});

	it('reports the chosen values in the order of the options', async () => {
		const { onchange, details } = renderPicker(['worker']);

		await userEvent.click(screen.getByText('Projects: 1'));
		expect(details.open).toBe(true);

		await userEvent.click(screen.getByRole('checkbox', { name: 'backend' }));
		expect(onchange).toHaveBeenLastCalledWith(['backend', 'worker']);

		await userEvent.click(screen.getByRole('checkbox', { name: 'All projects' }));
		expect(onchange).toHaveBeenLastCalledWith([]);
	});

	it('closes on Esc, which then does nothing else, and on a click outside', async () => {
		const { details, summary } = renderPicker();

		await userEvent.click(summary);
		expect(details.open).toBe(true);

		const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
		document.body.dispatchEvent(escape);
		expect(details.open).toBe(false);
		expect(escape.defaultPrevented).toBe(true);

		await userEvent.click(summary);
		await userEvent.click(document.body);
		expect(details.open).toBe(false);
	});

	it('is hidden with one option and nothing chosen', () => {
		const { details } = renderPicker([], ['backend']);

		expect(details).toBeNull();
	});
});
