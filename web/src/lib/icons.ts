// The one map from what the app shows to its Lucide icon; Icon.svelte draws them by name.
// One module per icon: the package's index would load every icon in the dev server.
import type { LucideIcon } from '@lucide/svelte';
import ArrowDown from '@lucide/svelte/icons/arrow-down';
import ArrowLeft from '@lucide/svelte/icons/arrow-left';
import ArrowUp from '@lucide/svelte/icons/arrow-up';
import ArrowUpDown from '@lucide/svelte/icons/arrow-up-down';
import Bell from '@lucide/svelte/icons/bell';
import Check from '@lucide/svelte/icons/check';
import ChevronDown from '@lucide/svelte/icons/chevron-down';
import ChevronRight from '@lucide/svelte/icons/chevron-right';
import CircleCheck from '@lucide/svelte/icons/circle-check';
import CircleQuestionMark from '@lucide/svelte/icons/circle-question-mark';
import CircleUserRound from '@lucide/svelte/icons/circle-user-round';
import CircleX from '@lucide/svelte/icons/circle-x';
import CodeXml from '@lucide/svelte/icons/code-xml';
import Copy from '@lucide/svelte/icons/copy';
import Ellipsis from '@lucide/svelte/icons/ellipsis';
import ExclamationMark from '@lucide/svelte/icons/exclamation-mark';
import ExternalLink from '@lucide/svelte/icons/external-link';
import Eye from '@lucide/svelte/icons/eye';
import EyeOff from '@lucide/svelte/icons/eye-off';
import Folder from '@lucide/svelte/icons/folder';
import History from '@lucide/svelte/icons/history';
import Info from '@lucide/svelte/icons/info';
import KeyRound from '@lucide/svelte/icons/key-round';
import LogOut from '@lucide/svelte/icons/log-out';
import LetterI from '@lucide/svelte/icons/letter-i';
import Logs from '@lucide/svelte/icons/logs';
import Monitor from '@lucide/svelte/icons/monitor';
import Moon from '@lucide/svelte/icons/moon';
import PanelLeftClose from '@lucide/svelte/icons/panel-left-close';
import PanelLeftOpen from '@lucide/svelte/icons/panel-left-open';
import Pause from '@lucide/svelte/icons/pause';
import PencilLine from '@lucide/svelte/icons/pencil-line';
import Play from '@lucide/svelte/icons/play';
import Plug from '@lucide/svelte/icons/plug';
import Plus from '@lucide/svelte/icons/plus';
import RotateCw from '@lucide/svelte/icons/rotate-cw';
import Send from '@lucide/svelte/icons/send';
import Settings from '@lucide/svelte/icons/settings';
import Sun from '@lucide/svelte/icons/sun';
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
	themeSystem: Monitor,
	themeLight: Sun,
	themeDark: Moon,
	sidebarCollapse: PanelLeftClose,
	sidebarExpand: PanelLeftOpen,
	add: Plus,
	edit: PencilLine,
	delete: Trash,
	more: Ellipsis,
	copy: Copy,
	check: Check,
	close: X,
	expand: ChevronDown,
	chevronRight: ChevronRight,
	back: ArrowLeft,
	sortable: ArrowUpDown,
	sortedAscending: ArrowUp,
	sortedDescending: ArrowDown,
	externalLink: ExternalLink,
	view: Eye,
	hide: EyeOff,
	settings: Settings,
	hint: CircleQuestionMark,
	token: KeyRound,
	code: CodeXml,
	pause: Pause,
	resume: Play,
	retry: RotateCw,
	history: History,
	send: Send,
	info: Info,
	success: CircleCheck,
	warning: TriangleExclamationPoint,
	error: CircleX,
	// The glyphs of a toast's round status mark.
	exclamation: ExclamationMark,
	letterI: LetterI
} satisfies Record<string, LucideIcon>;

export type IconName = keyof typeof icons;
