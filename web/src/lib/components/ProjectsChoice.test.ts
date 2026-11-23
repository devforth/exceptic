import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import ProjectsChoice from './ProjectsChoice.svelte';
import { projectsInput } from './projectsChoice.ts';

vi.mock('$app/state', () => ({ page: { data: {} } }));

const projects = [
	{ id: 1, name: 'backend' },
	{ id: 2, name: 'frontend' }
];

const summaryOf = (container: HTMLElement) => container.querySelector('summary');
const badges = (element: Element | null) =>
	[...(element?.querySelectorAll('[data-project]') ?? [])].map((badge) =>
		badge.textContent?.trim()
	);

describe('ProjectsChoice', () => {
	it('says All projects in the heading and lists no projects under it', () => {
		const { container } = render(ProjectsChoice, {
			projects,
			allProjects: true,
			projectIds: [],
			legend: 'Projects the user sees'
		});

		expect(summaryOf(container)?.textContent).toContain('All projects');
		expect(screen.queryByRole('checkbox', { name: 'backend' })).toBeNull();
		expect(screen.getByRole('group', { name: 'Projects the user sees' })).toBeTruthy();
	});

	it('lists the projects as badges once All is off, the chosen ones in the heading, none when empty', async () => {
		const user = userEvent.setup();
		const { container } = render(ProjectsChoice, {
			projects,
			allProjects: true,
			projectIds: [],
			legend: 'Projects'
		});

		await user.click(screen.getByRole('checkbox', { name: /All projects/ }));
		await user.click(screen.getByRole('checkbox', { name: 'frontend' }));
		await user.click(screen.getByRole('checkbox', { name: 'backend' }));

		// In the list's order, whatever the order of the clicks.
		expect(badges(summaryOf(container))).toEqual(['backend', 'frontend']);

		await user.click(screen.getByRole('checkbox', { name: 'backend' }));
		await user.click(screen.getByRole('checkbox', { name: 'frontend' }));
		expect(badges(summaryOf(container))).toEqual([]);
		expect(summaryOf(container)?.textContent).toContain('No projects');
	});

	it('offers All disabled with its reason when the viewer cannot give it', () => {
		render(ProjectsChoice, {
			projects,
			allProjects: false,
			projectIds: ['1'],
			legend: 'Projects',
			allowAll: false
		});

		const all = screen.getByRole<HTMLInputElement>('checkbox', { name: /All projects/ });

		expect(all.disabled).toBe(true);
		expect(all.checked).toBe(false);
		expect(screen.getByText(/Only someone with access to all projects can give it/)).toBeTruthy();
		expect(screen.getByRole<HTMLInputElement>('checkbox', { name: 'backend' }).checked).toBe(true);
		expect(screen.getByRole<HTMLInputElement>('checkbox', { name: 'frontend' }).disabled).toBe(
			false
		);
	});

	it('locks the whole field with its reason', () => {
		const { container } = render(ProjectsChoice, {
			projects,
			allProjects: true,
			projectIds: [],
			legend: 'Projects',
			locked: 'Only someone with access to all projects can change this.'
		});

		const all = screen.getByRole<HTMLInputElement>('checkbox', { name: /All projects/ });

		expect(all.disabled).toBe(true);
		expect(all.checked).toBe(true);
		expect(screen.getByText(/can change this/)).toBeTruthy();
		expect(summaryOf(container)?.textContent).toContain('All projects');
	});

	it('shows a refusal and opens', () => {
		const { container } = render(ProjectsChoice, {
			projects,
			allProjects: false,
			projectIds: [],
			legend: 'Projects',
			error: 'Unknown project.'
		});

		expect(screen.getByText('Unknown project.')).toBeTruthy();
		expect(container.querySelector('details')?.open).toBe(true);
	});
});

describe('projectsInput', () => {
	it('sends no projects with All', () => {
		expect(projectsInput(projects, true, ['1'])).toEqual({ all_projects: true, projects: [] });
	});

	it("sends the chosen ids in the list's order, without ones not listed", () => {
		expect(projectsInput(projects, false, ['2', '9', '1'])).toEqual({
			all_projects: false,
			projects: ['1', '2']
		});
	});
});
