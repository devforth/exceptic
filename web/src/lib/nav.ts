// The menu, and the addresses of the server-rendered UI: the one place the app names them
// until it replaces them. The Go server serves its pages at their addresses and the app at
// every other one; when a page moves here, its menu item gets a route instead of an href.
import type { IconName } from './icons.ts';

export const oldUI = {
	mcp: '/agents'
} as const;

/** An address with its query, encoded. */
export function withQuery(path: string, params: Record<string, string>): string {
	return `${path}?${new URLSearchParams(params).toString()}`;
}

/** The routes of the app that are in the menu. */
type AppRoute =
	| '/(app)/issues'
	| '/(app)/projects'
	| '/(app)/logs'
	| '/(app)/users'
	| '/(app)/notifications';

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
			{
				id: 'issues',
				label: 'Issues',
				icon: 'issues',
				route: '/(app)/issues',
				countsUnresolved: true
			},
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
				route: '/(app)/notifications',
				adminOnly: true
			},
			{ id: 'users', label: 'Users', icon: 'users', route: '/(app)/users', adminOnly: true },
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
