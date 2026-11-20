// The one map from what the app shows to its Lucide icon; Icon.svelte draws them by name.
// One module per icon: the package's index would load every icon in the dev server.
import type { LucideIcon } from '@lucide/svelte';
import ArrowLeft from '@lucide/svelte/icons/arrow-left';
import Bell from '@lucide/svelte/icons/bell';
import Check from '@lucide/svelte/icons/check';
import ChevronDown from '@lucide/svelte/icons/chevron-down';
import CircleCheck from '@lucide/svelte/icons/circle-check';
import CircleQuestionMark from '@lucide/svelte/icons/circle-question-mark';
import CircleUserRound from '@lucide/svelte/icons/circle-user-round';
import CircleX from '@lucide/svelte/icons/circle-x';
import CodeXml from '@lucide/svelte/icons/code-xml';
import Copy from '@lucide/svelte/icons/copy';
import Ellipsis from '@lucide/svelte/icons/ellipsis';
import ExternalLink from '@lucide/svelte/icons/external-link';
import Eye from '@lucide/svelte/icons/eye';
import Folder from '@lucide/svelte/icons/folder';
import KeyRound from '@lucide/svelte/icons/key-round';
import LogOut from '@lucide/svelte/icons/log-out';
import Logs from '@lucide/svelte/icons/logs';
import PanelLeftClose from '@lucide/svelte/icons/panel-left-close';
import PanelLeftOpen from '@lucide/svelte/icons/panel-left-open';
import Pause from '@lucide/svelte/icons/pause';
import PencilLine from '@lucide/svelte/icons/pencil-line';
import Plug from '@lucide/svelte/icons/plug';
import Plus from '@lucide/svelte/icons/plus';
import Settings from '@lucide/svelte/icons/settings';
import Trash from '@lucide/svelte/icons/trash';
import TriangleExclamationPoint from '@lucide/svelte/icons/triangle-exclamation-point';
import Users from '@lucide/svelte/icons/users';
import X from '@lucide/svelte/icons/x';

export const icons = {
	issues: TriangleExclamationPoint,
	logs: Logs,
	projects: Folder,
	notifications: Bell,
	users: Users,
	mcp: Plug,
	account: CircleUserRound,
	logout: LogOut,
	sidebarCollapse: PanelLeftClose,
	sidebarExpand: PanelLeftOpen,
	back: ArrowLeft,
	add: Plus,
	edit: PencilLine,
	delete: Trash,
	more: Ellipsis,
	copy: Copy,
	check: Check,
	close: X,
	expand: ChevronDown,
	externalLink: ExternalLink,
	view: Eye,
	settings: Settings,
	hint: CircleQuestionMark,
	token: KeyRound,
	code: CodeXml,
	pause: Pause,
	success: CircleCheck,
	warning: TriangleExclamationPoint,
	error: CircleX
} satisfies Record<string, LucideIcon>;

export type IconName = keyof typeof icons;
