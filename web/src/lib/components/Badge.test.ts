import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import Badge from './Badge.svelte';
import StatusBadge from './StatusBadge.svelte';
import StatusText from './StatusText.svelte';

vi.mock('$app/state', () => ({ page: { data: {} } }));

describe('StatusText', () => {
	it('draws a state as a pill with a dot hidden from screen readers', () => {
		const { container } = render(StatusText, {
			tone: 'success',
			children: createRawSnippet(() => ({ render: () => '<span>Active</span>' }))
		});

		expect(container.textContent?.trim()).toBe('Active');
		expect(container.querySelector('[aria-hidden="true"]')?.textContent).toBe('');
	});
});

describe('Badge', () => {
	it('names a project badge by its project', () => {
		const { container } = render(Badge, { tone: 'project', label: 'backend' });

		expect(container.querySelector('[data-project="backend"]')?.textContent?.trim()).toBe(
			'backend'
		);
	});
});

describe('StatusBadge', () => {
	it('is text with its meaning in the tooltip; "all" has no sign', () => {
		render(StatusBadge, { status: 'all' });

		expect(screen.getByText('all').getAttribute('title')).toBeNull();
		expect(screen.getByText('all').querySelector('[aria-hidden="true"]')).toBeNull();
	});
});
