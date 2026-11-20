// The menu, and the addresses of the server-rendered UI: the one place the app names them
// until it replaces them. The Go server serves its pages at their addresses and the app at
// every other one; when a page moves here, its menu item gets a route instead of an href.
import type { IconName } from './icons.ts';

export const oldUI = {
	issues: '/issues',
	issue: (id: string) => `/issues/${id}`,
	newProject: '/projects/new',
	project: (id: number) => `/projects/${id}`,
	cleanups: '/projects/cleanups',
	notifications: '/notifications',
	users: '/users',
	mcp: '/agents',
	account: '/account',
	login: '/login',
	logout: '/logout'
} as const;

/** An address with its query, encoded. */
export function withQuery(path: string, params: Record<string, string>): string {
	return `${path}?${new URLSearchParams(params).toString()}`;
}

/** The routes of the app that are in the menu. */
type AppRoute = '/(app)/projects' | '/(app)/logs';

export type NavEntry = {
	id: string;
	label: string;
	title?: string;
	icon: IconName;
	adminOnly?: boolean;
	/** The unresolved issues are counted on it. */
	countsUnresolved?: boolean;
} & ({ route: AppRoute; href?: never } | { href: string; route?: never });

export type NavGroup = { id: string; label: string; entries: NavEntry[] };

export const navGroups: NavGroup[] = [
	{
		id: 'monitoring',
		label: 'Monitoring',
		entries: [
			{ id: 'issues', label: 'Issues', icon: 'issues', href: oldUI.issues, countsUnresolved: true },
			{ id: 'logs', label: 'Logs', icon: 'logs', route: '/(app)/logs' }
		]
	},
	{
		id: 'settings',
		label: 'Settings',
		entries: [
			{ id: 'projects', label: 'Projects', icon: 'projects', route: '/(app)/projects' },
			{
				id: 'notifications',
				label: 'Notifications',
				icon: 'notifications',
				href: oldUI.notifications,
				adminOnly: true
			},
			{ id: 'users', label: 'Users', icon: 'users', href: oldUI.users, adminOnly: true },
			{
				id: 'mcp',
				label: 'MCP',
				title: 'MCP: connect a coding agent',
				icon: 'mcp',
				href: oldUI.mcp
			}
		]
	}
];
