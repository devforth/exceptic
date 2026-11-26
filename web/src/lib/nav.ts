// The menu of the app.
import type { IconName } from './icons.ts';

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
	| '/(app)/notifications'
	| '/(app)/agents';

export type NavEntry = {
	id: string;
	label: string;
	title?: string;
	icon: IconName;
	/** The unresolved issues are counted on it. */
	countsUnresolved?: boolean;
	route: AppRoute;
};

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
				route: '/(app)/notifications'
			},
			{ id: 'users', label: 'Users', icon: 'users', route: '/(app)/users' },
			{
				id: 'mcp',
				label: 'MCP',
				title: 'MCP: connect a coding agent',
				icon: 'mcp',
				route: '/(app)/agents'
			}
		]
	}
];
