// Generated from the catalog of internal/api by TestTypeScriptCatalog; do not edit: make api-types.

/** An RFC 3339 time in UTC. */
export type IsoTime = string;

/** Where the web app's requests go. */
export const apiPrefix = '/api/ui/';

/** The largest page of a list. */
export const maxLimit = 100;

/** Inputs and outputs of every request the web app may make. */
export interface Endpoints {
	confirm_email: {
		in: {
			/** The address the code was mailed to. */
			email: string;
			/** The 6-digit code from the email. */
			code: string;
		};
		out: {
			email: string;
			confirmed: boolean;
			message: string;
		};
	};
	count_logs: {
		in: {
			/** Project names or IDs; every project when empty. */
			project?: string[] | null;
			/** Log levels; every level when empty. */
			level?: ('trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal')[] | null;
			/** Text in the log line. */
			query?: string;
			/** Only logs of this trace (see get_issue). */
			trace_id?: string;
			/** RFC 3339 time: only logs at or after it. */
			since?: string;
			/** RFC 3339 time: only logs before it. */
			until?: string;
		};
		out: {
			count: number;
			more?: boolean;
		};
	};
	create_connection: {
		in: {
			/** email, telegram or slack. */
			type: string;
			/** email: an address confirmed with start_email_verification and confirm_email. */
			email?: string;
			/** A name; empty names it after the address or chat. */
			name?: string | null;
			/** Project names or IDs; an empty list means every project, including future ones. */
			projects?: string[] | null;
			/** Issue levels to notify about (default: the levels recorded by default). */
			levels?: ('debug' | 'info' | 'warning' | 'error' | 'fatal')[] | null;
			/** Send notifications (default true). */
			active?: boolean | null;
			/** Link to the issue in this panel (default true for email, false for chats, which may be public). */
			include_issue_link?: boolean | null;
			/** telegram: the bot token from @BotFather. */
			bot_token?: string;
			/** telegram: the chat ID (negative for groups) or @channel. */
			chat_id?: string;
			/** slack: an incoming webhook URL. */
			webhook_url?: string;
			/** slack: a bot token (xoxb-...) instead of a webhook; the bot also posts the agent prompt in a thread. */
			slack_bot_token?: string;
			/** slack: the channel ID the bot is invited to. */
			slack_channel?: string;
		};
		out: {
			id: string;
			name: string;
			type: string;
			email?: string;
			all_projects: boolean;
			projects?: string[] | null;
			levels: string[] | null;
			active: boolean;
			confirmed: boolean;
			include_issue_link: boolean;
			delivery?: {
				pending?: number;
				retrying?: number;
				failed?: number;
				last_sent_at?: IsoTime | null;
				last_error?: string;
			} | null;
		};
	};
	create_project: {
		in: {
			/** Lowercase letters, digits, '-' and '_', starting with a letter or digit, not digits only (that reads as an ID): backend, frontend, worker. */
			name: string;
			/** #rrggbb, or empty for the palette color. */
			color?: string | null;
			/** Event levels to store; an empty list stores none. */
			event_levels?: ('debug' | 'info' | 'warning' | 'error' | 'fatal')[] | null;
			/** Log levels to store; an empty list stores none. */
			log_levels?: ('trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal')[] | null;
		};
		out: {
			id: number;
			name: string;
			dsn?: string;
			from_env?: boolean;
			event_levels: string[] | null;
			log_levels: string[] | null;
			color?: string;
		};
	};
	create_user: {
		in: {
			/** Login name: letters, digits and . _ @ -. */
			username: string;
			/** Display name. */
			name?: string;
			/** admin or user. */
			role: string;
		};
		out: {
			user: {
				id: string;
				username: string;
				name?: string;
				role: string;
				must_change_password?: boolean;
				last_login_at?: IsoTime | null;
			};
			temporary_password: string;
			note: string;
		};
	};
	delete_connection: {
		in: {
			/** id from list_connections. */
			connection_id: string;
		};
		out: {
			done: string;
		};
	};
	delete_issue: {
		in: {
			/** Issue ID. */
			issue_id: string;
		};
		out: {
			deleted: string;
		};
	};
	delete_project: {
		in: {
			/** Project name or ID. */
			project: string;
		};
		out: {
			done: string;
		};
	};
	delete_user: {
		in: {
			/** id from list_users. */
			user_id: string;
		};
		out: {
			done: string;
		};
	};
	disconnect_agent: {
		in: {
			/** grant_id from list_agents. */
			grant_id: string;
		};
		out: {
			done: string;
		};
	};
	get_account: {
		in: Record<string, never>;
		out: {
			id: string;
			username: string;
			name?: string;
			role: string;
			must_change_password?: boolean;
			last_login_at?: IsoTime | null;
		};
	};
	get_event: {
		in: {
			/** Project name or ID. */
			project: string;
			/** Event ID: 32 hex characters, with or without dashes. */
			event_id: string;
		};
		out: string;
	};
	get_issue: {
		in: {
			/** Issue ID, as list_issues returns it. */
			issue_id: string;
			/** An event of this issue (see list_issue_events); its latest event when empty. */
			event_id?: string;
		};
		out: string;
	};
	get_issues: {
		in: {
			/** 1 to 100 known issue IDs. Duplicates are handled once. */
			issue_ids: string[] | null;
		};
		out: {
			issues: ({
				id: string;
				project: string;
				title: string;
				location?: string;
				level: string;
				status: string;
				event_count: number;
				first_seen: IsoTime;
				last_seen: IsoTime;
				resolved_at?: IsoTime | null;
			})[] | null;
			missing_issue_ids?: string[] | null;
		};
	};
	get_project: {
		in: {
			/** Project name or ID. */
			project: string;
		};
		out: {
			id: number;
			name: string;
			dsn?: string;
			from_env?: boolean;
			event_levels: string[] | null;
			log_levels: string[] | null;
			color?: string;
		};
	};
	get_session: {
		in: Record<string, never>;
		out: {
			user: {
				id: string;
				username: string;
				name?: string;
				role: string;
				must_change_password?: boolean;
				last_login_at?: IsoTime | null;
			};
			display_name: string;
			can_administer: boolean;
			app_name: string;
			unresolved_issues: number;
			project_colors_url: string;
		};
	};
	get_storage: {
		in: Record<string, never>;
		out: {
			used_bytes: number;
			used: string;
			limit_bytes?: number;
			limit?: string;
			percent?: number;
			cleanup_stop_percent: number;
		};
	};
	list_agents: {
		in: {
			/** Admins: a user they manage; the user the agent acts for when empty. */
			user_id?: string;
		};
		out: {
			user: string;
			agents: ({
				grant_id: string;
				name: string;
				kind: string;
				created_at: IsoTime;
				last_used_at?: IsoTime | null;
			})[] | null;
		};
	};
	list_connections: {
		in: Record<string, never>;
		out: {
			items: ({
				id: string;
				name: string;
				type: string;
				email?: string;
				all_projects: boolean;
				projects?: string[] | null;
				levels: string[] | null;
				active: boolean;
				confirmed: boolean;
				include_issue_link: boolean;
				delivery?: {
					pending?: number;
					retrying?: number;
					failed?: number;
					last_sent_at?: IsoTime | null;
					last_error?: string;
				} | null;
			})[] | null;
			next_cursor?: string;
		};
	};
	list_deliveries: {
		in: {
			/** Only this connection. */
			connection_id?: string;
			/** Delivery status; every status when empty. */
			status?: '' | 'pending' | 'sent' | 'failed';
			/** Messages per page. @default 20 */
			limit?: number;
			/** next_cursor of the previous page. */
			cursor?: string;
		};
		out: {
			items: {
				id: number;
				connection_id: string;
				message: string;
				project?: string;
				title?: string;
				issue_id?: string;
				status: string;
				attempts: number;
				last_error?: string;
				created_at: IsoTime;
				next_attempt_at: IsoTime;
			}[] | null;
			next_cursor?: string;
		};
	};
	list_issue_events: {
		in: {
			/** Issue ID. */
			issue_id: string;
			/** Events per page. @default 20 */
			limit?: number;
			/** next_cursor of the previous page. */
			cursor?: string;
		};
		out: {
			items: {
				event_id: string;
				timestamp: IsoTime;
				level: string;
				environment?: string;
				release?: string;
				trace_id?: string;
			}[] | null;
			next_cursor?: string;
		};
	};
	list_issues: {
		in: {
			/** Project names or IDs; every project when empty. */
			project?: string[] | null;
			/** Issue status, unresolved by default; all for every status. */
			status?: '' | 'unresolved' | 'resolved' | 'ignored' | 'all';
			/** Issue levels; every level when empty. */
			level?: ('debug' | 'info' | 'warning' | 'error' | 'fatal')[] | null;
			/** Text in the issue title or location. */
			query?: string;
			/** RFC 3339 time, e.g. 2026-11-11T10:00:00Z: only issues with an event at or after it. */
			since?: string;
			/** RFC 3339 time: only issues with an event before it. */
			until?: string;
			/** What since/until apply to; by default any event of the issue, its first or last seen time included. */
			date_by?: '' | 'event' | 'first_seen' | 'last_seen';
			/** Order, last_seen by default: a field newest or most first, with _asc the reverse; project A to Z, project_desc the reverse. */
			sort?: '' | 'project' | 'project_desc' | 'last_seen' | 'last_seen_asc' | 'first_seen' | 'first_seen_asc' | 'event_count' | 'event_count_asc';
			/** Issues per page. @default 20 */
			limit?: number;
			/** next_cursor of the previous page. */
			cursor?: string;
		};
		out: {
			items: ({
				id: string;
				project: string;
				title: string;
				location?: string;
				level: string;
				status: string;
				event_count: number;
				first_seen: IsoTime;
				last_seen: IsoTime;
				resolved_at?: IsoTime | null;
			})[] | null;
			next_cursor?: string;
		};
	};
	list_log_levels: {
		in: Record<string, never>;
		out: {
			levels: string[] | null;
		};
	};
	list_logs: {
		in: {
			/** Project names or IDs; every project when empty. */
			project?: string[] | null;
			/** Log levels; every level when empty. */
			level?: ('trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal')[] | null;
			/** Text in the log line. */
			query?: string;
			/** Only logs of this trace (see get_issue). */
			trace_id?: string;
			/** RFC 3339 time: only logs at or after it. */
			since?: string;
			/** RFC 3339 time: only logs before it. */
			until?: string;
			/** Logs per page. @default 20 */
			limit?: number;
			/** next_cursor of the previous page. */
			cursor?: string;
			/** Logs to skip, newest first, for numbered pages; next_cursor is faster. Not with cursor. */
			offset?: number;
		};
		out: {
			items: ({
				time: IsoTime;
				project: string;
				level: string;
				body: string;
				trace_id?: string;
				issue?: {
					id: string;
					title: string;
				} | null;
			})[] | null;
			next_cursor?: string;
		};
	};
	list_projects: {
		in: {
			/** Projects per page. @default 20 */
			limit?: number;
			/** next_cursor of the previous page. */
			cursor?: string;
		};
		out: {
			items: ({
				id: number;
				name: string;
				dsn?: string;
				from_env?: boolean;
				event_levels: string[] | null;
				log_levels: string[] | null;
				unresolved_issues: number;
			})[] | null;
			next_cursor?: string;
		};
	};
	list_users: {
		in: Record<string, never>;
		out: {
			items: ({
				id: string;
				username: string;
				name?: string;
				role: string;
				must_change_password?: boolean;
				last_login_at?: IsoTime | null;
			})[] | null;
			next_cursor?: string;
		};
	};
	reset_user_password: {
		in: {
			/** id from list_users. */
			user_id: string;
		};
		out: {
			user: {
				id: string;
				username: string;
				name?: string;
				role: string;
				must_change_password?: boolean;
				last_login_at?: IsoTime | null;
			};
			temporary_password: string;
			note: string;
		};
	};
	retry_connection: {
		in: {
			/** id from list_connections. */
			connection_id: string;
		};
		out: {
			done: string;
		};
	};
	send_test_message: {
		in: {
			/** id from list_connections. */
			connection_id: string;
		};
		out: {
			done: string;
		};
	};
	start_email_verification: {
		in: {
			/** The email address. */
			email: string;
		};
		out: {
			email: string;
			confirmed: boolean;
			message: string;
		};
	};
	update_account_name: {
		in: {
			/** The new display name; empty shows the username. */
			name: string;
		};
		out: {
			id: string;
			username: string;
			name?: string;
			role: string;
			must_change_password?: boolean;
			last_login_at?: IsoTime | null;
		};
	};
	update_connection: {
		in: {
			/** id from list_connections. */
			connection_id: string;
			/** A name; empty names it after the address or chat. */
			name?: string | null;
			/** Project names or IDs; an empty list means every project, including future ones. */
			projects?: string[] | null;
			/** Issue levels to notify about (default: the levels recorded by default). */
			levels?: ('debug' | 'info' | 'warning' | 'error' | 'fatal')[] | null;
			/** Send notifications (default true). */
			active?: boolean | null;
			/** Link to the issue in this panel (default true for email, false for chats, which may be public). */
			include_issue_link?: boolean | null;
			/** telegram: the bot token from @BotFather. */
			bot_token?: string;
			/** telegram: the chat ID (negative for groups) or @channel. */
			chat_id?: string;
			/** slack: an incoming webhook URL. */
			webhook_url?: string;
			/** slack: a bot token (xoxb-...) instead of a webhook; the bot also posts the agent prompt in a thread. */
			slack_bot_token?: string;
			/** slack: the channel ID the bot is invited to. */
			slack_channel?: string;
		};
		out: {
			id: string;
			name: string;
			type: string;
			email?: string;
			all_projects: boolean;
			projects?: string[] | null;
			levels: string[] | null;
			active: boolean;
			confirmed: boolean;
			include_issue_link: boolean;
			delivery?: {
				pending?: number;
				retrying?: number;
				failed?: number;
				last_sent_at?: IsoTime | null;
				last_error?: string;
			} | null;
		};
	};
	update_issue_status: {
		in: {
			/** 1 to 100 issue IDs; one ID for a single issue. */
			issue_ids: string[] | null;
			/** The status of every issue in the call. */
			status: 'unresolved' | 'resolved' | 'ignored';
		};
		out: {
			issues: ({
				id: string;
				project: string;
				title: string;
				location?: string;
				level: string;
				status: string;
				event_count: number;
				first_seen: IsoTime;
				last_seen: IsoTime;
				resolved_at?: IsoTime | null;
			})[] | null;
			missing_issue_ids?: string[] | null;
		};
	};
	update_project: {
		in: {
			/** Project name or ID. */
			project: string;
			/** A new name (not for projects from PROJECTS). */
			name?: string | null;
			/** #rrggbb, or empty for the palette color. */
			color?: string | null;
			/** Event levels to store; an empty list stores none. */
			event_levels?: ('debug' | 'info' | 'warning' | 'error' | 'fatal')[] | null;
			/** Log levels to store; an empty list stores none. */
			log_levels?: ('trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal')[] | null;
		};
		out: {
			id: number;
			name: string;
			dsn?: string;
			from_env?: boolean;
			event_levels: string[] | null;
			log_levels: string[] | null;
			color?: string;
		};
	};
	update_user: {
		in: {
			/** id from list_users. */
			user_id: string;
			/** Display name. */
			name?: string | null;
			/** admin or user. */
			role?: string | null;
		};
		out: {
			id: string;
			username: string;
			name?: string;
			role: string;
			must_change_password?: boolean;
			last_login_at?: IsoTime | null;
		};
	};
}

/** How each request is sent, and what the app must know before offering it. */
export const endpoints = {
	confirm_email: { method: 'POST', text: false, admin: true, destructive: false, confirm: false },
	count_logs: { method: 'GET', text: false, admin: false, destructive: false, confirm: false },
	create_connection: { method: 'POST', text: false, admin: true, destructive: false, confirm: true },
	create_project: { method: 'POST', text: false, admin: true, destructive: false, confirm: false },
	create_user: { method: 'POST', text: false, admin: true, destructive: false, confirm: true },
	delete_connection: { method: 'POST', text: false, admin: true, destructive: true, confirm: false },
	delete_issue: { method: 'POST', text: false, admin: true, destructive: true, confirm: false },
	delete_project: { method: 'POST', text: false, admin: true, destructive: true, confirm: false },
	delete_user: { method: 'POST', text: false, admin: true, destructive: true, confirm: false },
	disconnect_agent: { method: 'POST', text: false, admin: false, destructive: true, confirm: false },
	get_account: { method: 'GET', text: false, admin: false, destructive: false, confirm: false },
	get_event: { method: 'GET', text: true, admin: false, destructive: false, confirm: false },
	get_issue: { method: 'GET', text: true, admin: false, destructive: false, confirm: false },
	get_issues: { method: 'GET', text: false, admin: false, destructive: false, confirm: false },
	get_project: { method: 'GET', text: false, admin: true, destructive: false, confirm: false },
	get_session: { method: 'GET', text: false, admin: false, destructive: false, confirm: false },
	get_storage: { method: 'GET', text: false, admin: false, destructive: false, confirm: false },
	list_agents: { method: 'GET', text: false, admin: false, destructive: false, confirm: false },
	list_connections: { method: 'GET', text: false, admin: true, destructive: false, confirm: false },
	list_deliveries: { method: 'GET', text: false, admin: true, destructive: false, confirm: false },
	list_issue_events: { method: 'GET', text: false, admin: false, destructive: false, confirm: false },
	list_issues: { method: 'GET', text: false, admin: false, destructive: false, confirm: false },
	list_log_levels: { method: 'GET', text: false, admin: false, destructive: false, confirm: false },
	list_logs: { method: 'GET', text: false, admin: false, destructive: false, confirm: false },
	list_projects: { method: 'GET', text: false, admin: false, destructive: false, confirm: false },
	list_users: { method: 'GET', text: false, admin: true, destructive: false, confirm: false },
	reset_user_password: { method: 'POST', text: false, admin: true, destructive: true, confirm: false },
	retry_connection: { method: 'POST', text: false, admin: true, destructive: false, confirm: false },
	send_test_message: { method: 'POST', text: false, admin: true, destructive: false, confirm: false },
	start_email_verification: { method: 'POST', text: false, admin: true, destructive: false, confirm: false },
	update_account_name: { method: 'POST', text: false, admin: false, destructive: false, confirm: false },
	update_connection: { method: 'POST', text: false, admin: true, destructive: false, confirm: true },
	update_issue_status: { method: 'POST', text: false, admin: false, destructive: false, confirm: false },
	update_project: { method: 'POST', text: false, admin: true, destructive: false, confirm: true },
	update_user: { method: 'POST', text: false, admin: true, destructive: false, confirm: true },
} as const;

/** The values input fields take. */
export const choices = {
	count_logs: {
		level: ['trace', 'debug', 'info', 'warn', 'error', 'fatal'],
	},
	create_connection: {
		levels: ['debug', 'info', 'warning', 'error', 'fatal'],
	},
	create_project: {
		event_levels: ['debug', 'info', 'warning', 'error', 'fatal'],
		log_levels: ['trace', 'debug', 'info', 'warn', 'error', 'fatal'],
	},
	list_deliveries: {
		status: ['pending', 'sent', 'failed'],
	},
	list_issues: {
		date_by: ['event', 'first_seen', 'last_seen'],
		level: ['debug', 'info', 'warning', 'error', 'fatal'],
		sort: ['project', 'project_desc', 'last_seen', 'last_seen_asc', 'first_seen', 'first_seen_asc', 'event_count', 'event_count_asc'],
		status: ['unresolved', 'resolved', 'ignored', 'all'],
	},
	list_logs: {
		level: ['trace', 'debug', 'info', 'warn', 'error', 'fatal'],
	},
	update_connection: {
		levels: ['debug', 'info', 'warning', 'error', 'fatal'],
	},
	update_issue_status: {
		status: ['unresolved', 'resolved', 'ignored'],
	},
	update_project: {
		event_levels: ['debug', 'info', 'warning', 'error', 'fatal'],
		log_levels: ['trace', 'debug', 'info', 'warn', 'error', 'fatal'],
	},
} as const;

/** Event and log levels, highest first, as the app lists them. */
export const levels = {
	event: ['fatal', 'error', 'warning', 'info', 'debug'],
	log: ['fatal', 'error', 'warn', 'info', 'debug', 'trace']
} as const;

/** The color group a level is shown in. */
export const shownLevel = {
	debug: 'debug',
	info: 'info',
	warning: 'warning',
	error: 'error',
	fatal: 'error',
	trace: 'debug',
	warn: 'warning',
} as const;
