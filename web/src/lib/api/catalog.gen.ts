// Generated from the catalog of internal/api by TestTypeScriptCatalog; do not edit: make api-types.

/** An RFC 3339 time in UTC. */
export type IsoTime = string;

/** Where the web app's requests go. */
export const apiPrefix = '/api/ui/';

/** The largest page of a list. */
export const maxLimit = 100;

/** Inputs and outputs of every request the web app may make. */
export interface Endpoints {
	apply_issue_action: {
		in: {
			/** What to do with the issues. */
			action: 'resolve' | 'ignore' | 'reopen' | 'delete';
			/** The issues; issues deleted meanwhile are skipped. */
			issue_ids: string[] | null;
		};
		out: {
			action: string;
			issues: number;
		};
	};
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
			state: string;
			verified_at?: IsoTime | null;
			notice?: string;
			message: string;
		};
	};
	count_deliveries: {
		in: {
			/** Only this connection. */
			connection_id?: string;
			/** Delivery status; every status when empty. */
			status?: '' | 'pending' | 'sent' | 'failed';
		};
		out: {
			count: number;
			more?: boolean;
		};
	};
	count_issues: {
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
		};
		out: {
			count: number;
			more?: boolean;
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
			/** Every project, including future ones; false sends only for projects. */
			all_projects?: boolean | null;
			/** Project names or IDs; an empty list means every project, unless all_projects is false. */
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
			destination: string;
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
				failing?: boolean;
			} | null;
		};
	};
	create_project: {
		in: {
			/** Lowercase letters, digits, '-' and '_', starting with a letter or digit, not digits only (that reads as an ID): backend, frontend, worker. */
			name: string;
			/** #rrggbb, or empty for the palette color. */
			color?: string | null;
			/** Log levels to store; an empty list stores none. Events of every level are stored. */
			log_levels?: ('trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal')[] | null;
		};
		out: {
			id: number;
			name: string;
			dsn?: string;
			from_env?: boolean;
			log_levels: string[] | null;
			color?: string;
			shown_color: string;
		};
	};
	create_user: {
		in: {
			/** Login name: letters, digits and . _ @ -. */
			username: string;
			/** Display name. */
			name?: string;
			/** The role. */
			role: 'admin' | 'user';
		};
		out: {
			user: {
				id: string;
				username: string;
				name?: string;
				display_name: string;
				role: string;
				must_change_password?: boolean;
				created_at: IsoTime;
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
	get_connection: {
		in: {
			/** id from list_connections. */
			connection_id: string;
		};
		out: {
			id: string;
			name: string;
			type: string;
			email?: string;
			confirmed: boolean;
			verified_at?: IsoTime | null;
			all_projects: boolean;
			project_ids: number[] | null;
			levels: string[] | null;
			active: boolean;
			include_issue_link: boolean;
			chat_id?: string;
			slack_channel?: string;
			saved_telegram_token?: string;
			saved_webhook_url?: string;
			saved_slack_bot_token?: string;
			secret_readable: boolean;
			retrying: number;
		};
	};
	get_connection_defaults: {
		in: Record<string, never>;
		out: {
			types: {
				type: string;
				available: boolean;
				reason?: string;
				include_issue_link: boolean;
				name_placeholder: string;
			}[] | null;
			default_type: string;
			levels: string[] | null;
			issue_links_available: boolean;
		};
	};
	get_issue_details: {
		in: {
			/** Issue ID. */
			issue_id: string;
			/** An event of this issue (UUID or 32 hex); the latest when empty. */
			event_id?: string;
		};
		out: {
			issue: {
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
				mcp_reference?: string;
			};
			agent_prompt: string;
			history: {
				from: string;
				to: string;
				name?: string;
				by_event?: boolean;
				via_mcp?: boolean;
				event_id?: string;
				created_at: IsoTime;
			}[] | null;
			events: {
				items: {
					event_id: string;
					timestamp: IsoTime;
					level: string;
					environment?: string;
					release?: string;
					trace_id?: string;
					received_at: IsoTime;
				}[] | null;
				next_cursor?: string;
			};
			event_state: 'shown' | 'unreadable' | 'missing' | 'none';
			event?: {
				event_id: string;
				timestamp: IsoTime;
				clock_drift?: string;
				thinned?: boolean;
				platform?: string;
				environment?: string;
				release?: string;
				server_name?: string;
				sdk?: string;
				trace_id?: string;
				trace_has_logs?: boolean;
				message?: string;
				exceptions: ({
					type?: string;
					value?: string;
					handled?: boolean | null;
					frames: ({
						function?: string;
						location?: string;
						line?: number;
						column?: number;
						in_app?: boolean;
						source_lines: {
							number: number;
							text: string;
							current?: boolean;
						}[] | null;
						vars?: string;
					})[] | null;
				})[] | null;
				breadcrumbs: ({
					timestamp?: IsoTime | null;
					category?: string;
					level?: string;
					message?: string;
					data?: string;
				})[] | null;
				request?: {
					method?: string;
					url?: string;
					headers: {
						name: string;
						value: string;
					}[] | null;
				} | null;
				tags: {
					name: string;
					value: string;
				}[] | null;
				user?: {
					id?: string;
					email?: string;
					username?: string;
					ip_address?: string;
				} | null;
				contexts: {
					name: string;
					value: string;
				}[] | null;
				related_logs: {
					time: IsoTime;
					level: string;
					body: string;
				}[] | null;
				logs_from_request?: boolean;
				payload: string;
			} | null;
		};
	};
	get_mcp_setup: {
		in: Record<string, never>;
		out: {
			url: string;
			server_name: string;
			claude_command: string;
			codex_command: string;
			project_config: string;
			token_set: boolean;
			unencrypted: boolean;
			ips_restricted: boolean;
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
			log_levels: string[] | null;
			color?: string;
			shown_color: string;
		};
	};
	get_project_defaults: {
		in: Record<string, never>;
		out: {
			shown_color: string;
			log_levels: string[] | null;
		};
	};
	get_project_setup: {
		in: {
			/** Project ID. */
			project: number;
		};
		out: {
			id: number;
			name: string;
			dsn: string;
			received: boolean;
			platforms: {
				id: string;
				label: string;
				install: string;
				where: string;
				init: string;
				test: string;
				agent_prompt: string;
			}[] | null;
		};
	};
	get_session: {
		in: Record<string, never>;
		out: {
			user: {
				id: string;
				username: string;
				name?: string;
				display_name: string;
				role: string;
				must_change_password?: boolean;
				created_at: IsoTime;
				last_login_at?: IsoTime | null;
			};
			can_administer: boolean;
			app_name: string;
			unresolved_issues: number;
			project_colors: Record<string, string>;
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
			projects: ({
				id: number;
				name: string;
				bytes: number;
				size?: string;
				last_received?: IsoTime | null;
			})[] | null;
			other_bytes: number;
			other: string;
			counting?: boolean;
		};
	};
	get_user: {
		in: {
			/** id from list_users. */
			user_id: string;
		};
		out: {
			user: {
				id: string;
				username: string;
				name?: string;
				display_name: string;
				role: string;
				must_change_password?: boolean;
				created_at: IsoTime;
				last_login_at?: IsoTime | null;
			};
			can_manage: boolean;
		};
	};
	list_agents: {
		in: {
			/** Admins: a user they manage; the user the agent acts for when empty. */
			user_id?: string;
		};
		out: {
			user: string;
			user_id: string;
			owners?: {
				id: string;
				name: string;
			}[] | null;
			agents: ({
				grant_id: string;
				name: string;
				kind: string;
				created_at: IsoTime;
				last_used_at?: IsoTime | null;
				connected: boolean;
			})[] | null;
		};
	};
	list_cleanups: {
		in: Record<string, never>;
		out: {
			stop_percent: number;
			kept: number;
			runs: ({
				id: number;
				started_at: IsoTime;
				status: string;
				limit: string;
				size_before: string;
				compacted: string;
				size_after: string;
				stop: string;
				needed: string;
				deleted_steps: string;
				result: string;
				took: string;
				error?: string;
				steps: ({
					number: number;
					name: string;
					freed: string;
					took: string;
					projects: {
						project: string;
						deleted: string;
					}[] | null;
				})[] | null;
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
				destination: string;
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
					failing?: boolean;
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
			items: ({
				id: number;
				connection_id: string;
				message: string;
				project?: string;
				title?: string;
				issue_id?: string;
				items?: {
					message: string;
					project?: string;
					title?: string;
					issue_id?: string;
				}[] | null;
				more?: number;
				status: string;
				retrying: boolean;
				attempts: number;
				last_error?: string;
				created_at: IsoTime;
				updated_at: IsoTime;
				next_attempt_at: IsoTime;
			})[] | null;
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
				received_at: IsoTime;
			}[] | null;
			next_cursor?: string;
		};
	};
	list_issue_levels: {
		in: Record<string, never>;
		out: {
			levels: string[] | null;
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
				mcp_reference?: string;
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
				user: {
					id: string;
					username: string;
					name?: string;
					display_name: string;
					role: string;
					must_change_password?: boolean;
					created_at: IsoTime;
					last_login_at?: IsoTime | null;
				};
				can_manage: boolean;
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
				display_name: string;
				role: string;
				must_change_password?: boolean;
				created_at: IsoTime;
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
	set_connection_active: {
		in: {
			/** id from list_connections. */
			connection_id: string;
			/** true sends notifications, false stops them. */
			active: boolean;
		};
		out: {
			id: string;
			name: string;
			type: string;
			email?: string;
			destination: string;
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
				failing?: boolean;
			} | null;
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
			state: string;
			verified_at?: IsoTime | null;
			notice?: string;
			message: string;
		};
	};
	test_chat: {
		in: {
			/** telegram or slack; with connection_id, its own type. */
			type?: string;
			/** A saved connection: its tokens fill the empty fields. */
			connection_id?: string;
			/** The name the test message shows. */
			name?: string;
			/** As the form has it. */
			include_issue_link?: boolean;
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
			sent: boolean;
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
			display_name: string;
			role: string;
			must_change_password?: boolean;
			created_at: IsoTime;
			last_login_at?: IsoTime | null;
		};
	};
	update_connection: {
		in: {
			/** id from list_connections. */
			connection_id: string;
			/** A name; empty names it after the address or chat. */
			name?: string | null;
			/** Every project, including future ones; false sends only for projects. */
			all_projects?: boolean | null;
			/** Project names or IDs; an empty list means every project, unless all_projects is false. */
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
			destination: string;
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
				failing?: boolean;
			} | null;
		};
	};
	update_project: {
		in: {
			/** Project name or ID. */
			project: string;
			/** A new name. */
			name?: string | null;
			/** #rrggbb, or empty for the palette color. */
			color?: string | null;
			/** Log levels to store; an empty list stores none. Events of every level are stored. */
			log_levels?: ('trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal')[] | null;
		};
		out: {
			id: number;
			name: string;
			dsn?: string;
			from_env?: boolean;
			log_levels: string[] | null;
			color?: string;
			shown_color: string;
		};
	};
	update_user: {
		in: {
			/** id from list_users. */
			user_id: string;
			/** Display name. */
			name?: string | null;
			/** The role. */
			role?: '' | 'admin' | 'user';
		};
		out: {
			id: string;
			username: string;
			name?: string;
			display_name: string;
			role: string;
			must_change_password?: boolean;
			created_at: IsoTime;
			last_login_at?: IsoTime | null;
		};
	};
}

/** How each request is sent, and what the app must know before offering it. */
export const endpoints = {
	apply_issue_action: { method: 'POST', admin: false, destructive: false, confirm: false },
	confirm_email: { method: 'POST', admin: true, destructive: false, confirm: false },
	count_deliveries: { method: 'GET', admin: true, destructive: false, confirm: false },
	count_issues: { method: 'GET', admin: false, destructive: false, confirm: false },
	count_logs: { method: 'GET', admin: false, destructive: false, confirm: false },
	create_connection: { method: 'POST', admin: true, destructive: false, confirm: true },
	create_project: { method: 'POST', admin: true, destructive: false, confirm: false },
	create_user: { method: 'POST', admin: true, destructive: false, confirm: true },
	delete_connection: { method: 'POST', admin: true, destructive: true, confirm: false },
	delete_project: { method: 'POST', admin: true, destructive: true, confirm: false },
	delete_user: { method: 'POST', admin: true, destructive: true, confirm: false },
	disconnect_agent: { method: 'POST', admin: false, destructive: true, confirm: false },
	get_connection: { method: 'GET', admin: true, destructive: false, confirm: false },
	get_connection_defaults: { method: 'GET', admin: true, destructive: false, confirm: false },
	get_issue_details: { method: 'GET', admin: false, destructive: false, confirm: false },
	get_mcp_setup: { method: 'GET', admin: false, destructive: false, confirm: false },
	get_project: { method: 'GET', admin: true, destructive: false, confirm: false },
	get_project_defaults: { method: 'GET', admin: true, destructive: false, confirm: false },
	get_project_setup: { method: 'GET', admin: false, destructive: false, confirm: false },
	get_session: { method: 'GET', admin: false, destructive: false, confirm: false },
	get_storage: { method: 'GET', admin: false, destructive: false, confirm: false },
	get_user: { method: 'GET', admin: true, destructive: false, confirm: false },
	list_agents: { method: 'GET', admin: false, destructive: false, confirm: false },
	list_cleanups: { method: 'GET', admin: false, destructive: false, confirm: false },
	list_connections: { method: 'GET', admin: true, destructive: false, confirm: false },
	list_deliveries: { method: 'GET', admin: true, destructive: false, confirm: false },
	list_issue_events: { method: 'GET', admin: false, destructive: false, confirm: false },
	list_issue_levels: { method: 'GET', admin: false, destructive: false, confirm: false },
	list_issues: { method: 'GET', admin: false, destructive: false, confirm: false },
	list_log_levels: { method: 'GET', admin: false, destructive: false, confirm: false },
	list_logs: { method: 'GET', admin: false, destructive: false, confirm: false },
	list_projects: { method: 'GET', admin: false, destructive: false, confirm: false },
	list_users: { method: 'GET', admin: true, destructive: false, confirm: false },
	reset_user_password: { method: 'POST', admin: true, destructive: true, confirm: false },
	retry_connection: { method: 'POST', admin: true, destructive: false, confirm: false },
	send_test_message: { method: 'POST', admin: true, destructive: false, confirm: false },
	set_connection_active: { method: 'POST', admin: true, destructive: false, confirm: true },
	start_email_verification: { method: 'POST', admin: true, destructive: false, confirm: false },
	test_chat: { method: 'POST', admin: true, destructive: false, confirm: false },
	update_account_name: { method: 'POST', admin: false, destructive: false, confirm: false },
	update_connection: { method: 'POST', admin: true, destructive: false, confirm: true },
	update_project: { method: 'POST', admin: true, destructive: false, confirm: true },
	update_user: { method: 'POST', admin: true, destructive: false, confirm: true },
} as const;

/** Inputs and outputs of the plain requests: logins and credentials, which no endpoint handles. */
export interface PlainRequests {
	login_state: {
		in: {
			next?: string;
		};
		out: {
			app_name: string;
			disabled?: string;
			redirect?: string;
		};
	};
	login: {
		in: {
			username: string;
			password: string;
			next?: string;
		};
		out: {
			next: string;
		};
	};
	logout: {
		in: Record<string, never>;
		out: void;
	};
	change_password: {
		in: {
			current_password: string;
			password: string;
			confirm_password: string;
		};
		out: void;
	};
	personal_token: {
		in: Record<string, never>;
		out: {
			token: string;
			setup_prompt: string;
		};
	};
	authorization_request: {
		in: {
			query: string;
		};
		out: {
			client_name: string;
			client_site?: string;
			redirect_uri: string;
			form_token: string;
		};
	};
	authorization_decision: {
		in: {
			query: string;
			form_token: string;
			allow: boolean;
		};
		out: {
			redirect_url: string;
		};
	};
}

/** How each plain request is sent. */
export const plainRequests = {
	login_state: { method: 'GET' },
	login: { method: 'POST' },
	logout: { method: 'POST' },
	change_password: { method: 'POST' },
	personal_token: { method: 'POST' },
	authorization_request: { method: 'GET' },
	authorization_decision: { method: 'POST' },
} as const;

/** The answer to a refused request: fields by input name, the record that already exists. */
export type ErrorBody = {
	error: string;
	fields?: Record<string, string>;
	duplicate?: {
		id: string;
		name: string;
	} | null;
};

/** The values input fields take. */
export const choices = {
	apply_issue_action: {
		action: ['resolve', 'ignore', 'reopen', 'delete'],
	},
	count_deliveries: {
		status: ['pending', 'sent', 'failed'],
	},
	count_issues: {
		date_by: ['event', 'first_seen', 'last_seen'],
		level: ['debug', 'info', 'warning', 'error', 'fatal'],
		status: ['unresolved', 'resolved', 'ignored', 'all'],
	},
	count_logs: {
		level: ['trace', 'debug', 'info', 'warn', 'error', 'fatal'],
	},
	create_connection: {
		levels: ['debug', 'info', 'warning', 'error', 'fatal'],
	},
	create_project: {
		log_levels: ['trace', 'debug', 'info', 'warn', 'error', 'fatal'],
	},
	create_user: {
		role: ['admin', 'user'],
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
	update_project: {
		log_levels: ['trace', 'debug', 'info', 'warn', 'error', 'fatal'],
	},
	update_user: {
		role: ['admin', 'user'],
	},
} as const;

/** Event and log levels, highest first, as the app lists them. */
export const levels = {
	event: ['fatal', 'error', 'warning', 'info', 'debug'],
	log: ['fatal', 'error', 'warn', 'info', 'debug', 'trace']
} as const;

/** The color group a level is shown in, the other names SDKs use included. */
export const shownLevel = {
	debug: 'debug',
	info: 'info',
	warning: 'warning',
	error: 'error',
	fatal: 'error',
	trace: 'debug',
	warn: 'warning',
	critical: 'error',
	log: 'info',
} as const;

/** The limits of form fields, as the server checks them; a pattern must match the whole value. */
export const limits = {
	connectionName: 100,
	username: 64,
	usernamePattern: '[A-Za-z0-9._@\\-]+',
	displayName: 100,
	minPassword: 12,
	maxPassword: 256,
	projectName: 64,
	projectNamePattern: '[a-z][a-z0-9_\\-]*|[0-9][a-z0-9_\\-]*[a-z_\\-][a-z0-9_\\-]*',
	verificationCode: 6,
	telegramToken: '\\d+:[A-Za-z0-9_\\-]{20,}',
	telegramChat: '-?\\d+|@[A-Za-z0-9_]{5,}',
	slackToken: 'xoxb-[A-Za-z0-9\\-]{10,}',
	slackChannel: '#?[A-Za-z0-9._\\-]{1,80}',
} as const;

/** Issue statuses in the order of the status filter, with what each means. */
export const issueStatuses = [
	{ name: 'unresolved', meaning: 'still happening or not looked at yet' },
	{ name: 'resolved', meaning: 'fixed, reopens if the error happens again' },
	{ name: 'ignored', meaning: 'no notifications, stays ignored' },
] as const;

/** The most issues one action takes. */
export const maxActionIssues = 1000;

/** The kind of a connected agent that is a personal token (list_agents). */
export const personalTokenKind = 'token';

/** The palette of project colors: a project follows it by its ID until it has its own color. */
export const projectPalette = [
	{ name: 'Blue', color: '#2563eb' },
	{ name: 'Pink', color: '#be185d' },
	{ name: 'Green', color: '#15803d' },
	{ name: 'Violet', color: '#7c3aed' },
	{ name: 'Orange', color: '#c2410c' },
	{ name: 'Teal', color: '#0f766e' },
	{ name: 'Indigo', color: '#4338ca' },
	{ name: 'Amber', color: '#a16207' },
	{ name: 'Cyan', color: '#0e7490' },
	{ name: 'Fuchsia', color: '#a21caf' },
] as const;

/** What each role may do. */
export const roleSummaries = {
	admin: 'projects, notifications, users and deleting issues',
	superadmin: 'everything',
	user: 'issue statuses, logs, project DSNs and their own coding agents',
} as const;

/** The help of the settings pages: docs/ui.md by section and field label, docs/configuration.md by variable. */
export const help = {
	fields: {
		'Notifications': {
			'Active': 'Off: nothing is sent, and messages waiting for a retry fail.',
			'Channel ID': 'The channel the Slack bot posts to (C0123456789, at the bottom of the channel details). Invite the bot to it first.',
			'Chat ID': 'The chat the bot writes to: a number (negative for groups; supergroups and channels start with -100) or @channel_name of a public channel. To find the number, write a message in the chat and press Find chat ID: it is chat.id in the answer.',
			'Email address': 'Gets a 6-digit code first; the connection sends nothing until the code is entered. An address confirmed before is accepted at once.',
			'Include Open issue link': 'Adds a link to the issue page; offered only when PUBLIC_URL is set. Off by default for chats, which people outside the team may read.',
			'Incoming webhook URL': 'A Slack incoming webhook; it posts to the channel chosen when it was created. Mattermost and Rocket.Chat webhooks work too. Stored encrypted.',
			'Levels': 'Levels that are sent; a new connection sends fatal and error. Events of other levels are still stored, they only do not notify.',
			'Projects': 'Projects whose issues are sent. All projects includes the ones added later.',
			'Slack bot token': 'Optional, instead of the webhook: a bot token (xoxb-…) with the chat:write scope. The bot posts each error and puts the prompt for a coding agent in a thread under it. A new token replaces a saved webhook, and a new webhook without a token replaces a saved bot and its channel. Stored encrypted.',
			'Telegram bot token': 'The token @BotFather gives for your bot. The bot must be a member of the chat. Stored encrypted.',
			'Type': 'Email needs a mail server (SMTP_HOST); Telegram and Slack need only the tokens entered below.',
		},
		'Projects': {
			'Color': 'Marks the project\'s badge in lists and the header of its emails.',
			'DSN': 'The address the app\'s Sentry SDK sends errors and logs to. The key in it only allows sending to this project, so it can be in browser code.',
			'Logs': 'Log levels to store; others are dropped when they arrive. Unchanged, the project follows RECORD_LOG_LEVELS. Errors and messages of every level are stored.',
		},
	},
	settings: {
		MAX_DATABASE_SIZE: 'Once the database reaches it, the least useful data is deleted until the database is at 80%. Takes KB, MB, GB, at least 500MB; 0 turns the limit off, and nothing is deleted or thinned. Envelopes waiting to be stored may take 20% of it (of 2GB when it is off); beyond that new ones get 429 until the queue drains.',
	}
} as const;
