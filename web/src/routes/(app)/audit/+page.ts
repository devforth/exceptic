import { get, listAll, pageError } from '#lib/api/client.ts';
import { pageWithin, readPaging } from '#lib/filters.ts';
import type { PageLoad } from './$types';
import { auditChoices, auditInput, readAuditFilters } from './audit.ts';

export const load: PageLoad = async ({ url }) => {
	const params = url.searchParams;
	// The users the viewer sees (their names); hidden ones show in the log as Admin or User. The
	// filters offer only what some entry has (list_audit_choices).
	const [projects, listed, present] = await Promise.all([
		listAll('list_projects'),
		get('list_users'),
		get('list_audit_choices')
	]).catch(pageError);
	const filters = readAuditFilters(params, projects);
	const input = auditInput(filters);
	const paging = readPaging(params);
	const count = await get('count_audit', input).catch(pageError);
	// A page past the end shows the last one.
	const page = pageWithin(paging, count.count);
	const entries =
		count.count === 0
			? undefined
			: await get('list_audit', {
					...input,
					limit: paging.perPage,
					cursor: String((page - 1) * paging.perPage)
				}).catch(pageError);

	const users = (listed.items ?? []).map(({ user }) => ({
		id: user.id,
		name: user.display_name,
		email: user.email
	}));

	return {
		filters,
		choices: auditChoices(
			present,
			filters,
			projects.map((project) => project.name),
			users.map((user) => user.id)
		),
		users,
		count,
		page,
		perPage: paging.perPage,
		entries: entries?.items ?? []
	};
};
