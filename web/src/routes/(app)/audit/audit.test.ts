import { describe, expect, it } from 'vitest';
import { auditChoices, auditInput, readAuditFilters } from './audit.ts';

const projects = [
	{ id: 1, name: 'backend' },
	{ id: 2, name: 'frontend' }
];

describe('audit filters', () => {
	it('keeps only known records, actions and projects, in their own order', () => {
		const params = new URLSearchParams(
			'resource=user,bogus&resource=issue&action=delete&action=drop&project=2&project=gone&user=k3x9q2mf&query=+ann+&from=2026-11-20&to=2026-11-10'
		);
		const filters = readAuditFilters(params, projects);

		expect(filters).toEqual({
			resource: ['issue', 'user'],
			action: ['delete'],
			project: ['frontend'],
			user: ['k3x9q2mf'],
			query: 'ann',
			from: '2026-11-10',
			to: '2026-11-20'
		});
		expect(auditInput(filters)).toEqual({
			resource: ['issue', 'user'],
			action: ['delete'],
			project: ['frontend'],
			user_id: ['k3x9q2mf'],
			query: 'ann',
			since: '2026-11-10T00:00:00Z',
			until: '2026-11-21T00:00:00.000Z'
		});
	});

	it('asks for everything without filters', () => {
		const filters = readAuditFilters(new URLSearchParams(), projects);

		expect(auditInput(filters)).toEqual({
			resource: [],
			action: [],
			project: [],
			user_id: [],
			query: undefined
		});
	});

	it('offers what some entry has and what the address chose, in their own order', () => {
		const filters = readAuditFilters(
			new URLSearchParams('resource=project&project=frontend&user=gone0000'),
			projects
		);
		const present = {
			resources: ['issue', 'user'],
			actions: ['delete'],
			user_ids: ['b8d3e5f0'],
			projects: ['backend']
		};

		expect(
			auditChoices(present, filters, ['backend', 'frontend', 'quiet'], ['a7c2d4e9', 'b8d3e5f0'])
		).toEqual({
			resource: ['issue', 'project', 'user'],
			action: ['delete'],
			project: ['backend', 'frontend'],
			user: ['b8d3e5f0', 'gone0000']
		});
	});
});
