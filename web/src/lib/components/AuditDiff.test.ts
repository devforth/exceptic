import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import AuditDiff from './AuditDiff.svelte';

describe('AuditDiff', () => {
	it('shows each changed field as a diff: before in red over after in green, the change marked', () => {
		render(AuditDiff, {
			before: { status: 'unresolved', name: 'api' },
			after: { status: 'resolved', name: 'api-v2' }
		});

		const row = screen.getByRole('rowheader', { name: 'name' }).closest('tr')!;
		const before = row.querySelector<HTMLElement>('[data-side="before"]')!;
		const after = row.querySelector<HTMLElement>('[data-side="after"]')!;

		expect(before.textContent).toContain('api');
		expect(before.className).toContain('text-error');
		expect(after.className).toContain('text-success');
		// Only what was added is marked: "api" is common, "-v2" new.
		expect(after.querySelector('mark')?.textContent).toBe('-v2');
		expect(before.querySelector('mark')).toBeNull();
	});

	it('shows only the after lines of a create', () => {
		const { container } = render(AuditDiff, { after: { name: 'backend' } });

		expect(container.querySelector('[data-side="before"]')).toBeNull();
		expect(container.querySelector('[data-side="after"]')?.textContent).toContain('backend');
	});

	it('says when no field changed', () => {
		render(AuditDiff, {});

		expect(screen.getByText('No field changed.')).toBeTruthy();
	});
});
