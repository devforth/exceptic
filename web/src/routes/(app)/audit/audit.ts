// The audit log page: its filters, read from the address, and the request they make.
import { choices, type Endpoints } from '#lib/api/catalog.gen.ts';
import {
	dayRange,
	offered,
	readChoices,
	readDays,
	readList,
	readProjects,
	readText
} from '#lib/filters.ts';

type Resource = (typeof choices.list_audit.resource)[number];
type Action = (typeof choices.list_audit.action)[number];

export type AuditFilters = {
	resource: Resource[];
	action: Action[];
	project: string[];
	/** Users' ids: only their changes. */
	user: string[];
	query: string;
	from: string;
	to: string;
};

/** ?resource=…&action=…&project=…&user=…&query=…&from=…&to=…: only known values are kept. */
export function readAuditFilters(
	params: URLSearchParams,
	projects: readonly { id: number; name: string }[]
): AuditFilters {
	return {
		resource: readChoices(params, 'resource', choices.list_audit.resource),
		action: readChoices(params, 'action', choices.list_audit.action),
		project: readProjects(params, projects),
		user: readList(params, 'user'),
		query: readText(params, 'query'),
		...readDays(params)
	};
}

/** What count_audit and list_audit are asked for these filters. */
export function auditInput(filters: AuditFilters): Endpoints['count_audit']['in'] {
	return {
		resource: filters.resource,
		action: filters.action,
		project: filters.project,
		user_id: filters.user,
		query: filters.query || undefined,
		...dayRange(filters.from, filters.to)
	};
}

/**
 * What the filters offer: the values some entry the viewer sees has (list_audit_choices), and
 * whatever the address chose. Users are those the viewer sees with an entry, then chosen ones they
 * do not list (a user deleted since).
 */
export function auditChoices(
	present: Endpoints['list_audit_choices']['out'],
	filters: AuditFilters,
	projectNames: readonly string[],
	userIds: readonly string[]
) {
	return {
		resource: offered(choices.list_audit.resource, present.resources, filters.resource),
		action: offered(choices.list_audit.action, present.actions, filters.action),
		project: offered(projectNames, present.projects, filters.project),
		user: [
			...offered(userIds, present.user_ids, filters.user),
			...filters.user.filter((id) => !userIds.includes(id))
		]
	};
}
