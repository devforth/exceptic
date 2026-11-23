<!--
@component
Every component with its variants, on sample data. Served only by the dev server
(routes/design-system); the build test fails if the marker id below reaches dist.
-->
<script lang="ts">
	import { help, levels, projectPalette } from '#lib/api/catalog.gen.ts';
	import { ApiError } from '#lib/api/client.ts';
	import { FormState } from '#lib/form.svelte.ts';
	import Badge from '#lib/components/Badge.svelte';
	import type { BadgeTone } from '#lib/components/badge.ts';
	import BulkBar from '#lib/components/BulkBar.svelte';
	import Button from '#lib/components/Button.svelte';
	import type { ButtonVariant } from '#lib/components/button.ts';
	import ActiveFilters from '#lib/components/ActiveFilters.svelte';
	import AgentCopyButton from '#lib/components/AgentCopyButton.svelte';
	import BackButton from '#lib/components/BackButton.svelte';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import Card from '#lib/components/Card.svelte';
	import Choice from '#lib/components/Choice.svelte';
	import ChoiceGroup from '#lib/components/ChoiceGroup.svelte';
	import CodeBlock from '#lib/components/CodeBlock.svelte';
	import ColorField from '#lib/components/ColorField.svelte';
	import type { Confirmation } from '#lib/components/confirm.ts';
	import ConfirmButton from '#lib/components/ConfirmButton.svelte';
	import CopyButton from '#lib/components/CopyButton.svelte';
	import CopyField from '#lib/components/CopyField.svelte';
	import DateRange from '#lib/components/DateRange.svelte';
	import Details from '#lib/components/Details.svelte';
	import EditableTitle from '#lib/components/EditableTitle.svelte';
	import Dialog from '#lib/components/Dialog.svelte';
	import FormDialog from '#lib/components/FormDialog.svelte';
	import SubmitButton from '#lib/components/SubmitButton.svelte';
	import Facts from '#lib/components/Facts.svelte';
	import FilterBar from '#lib/components/FilterBar.svelte';
	import FormGroup from '#lib/components/FormGroup.svelte';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import Hint from '#lib/components/Hint.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import IssueActions from '#lib/components/IssueActions.svelte';
	import LevelBadge from '#lib/components/LevelBadge.svelte';
	import LevelChoices from '#lib/components/LevelChoices.svelte';
	import LevelSummary from '#lib/components/LevelSummary.svelte';
	import LogBody from '#lib/components/LogBody.svelte';
	import type { MenuItem } from '#lib/components/menu.ts';
	import NavLink from '#lib/components/NavLink.svelte';
	import NotFound from '#lib/components/NotFound.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import Pager from '#lib/components/Pager.svelte';
	import Picker from '#lib/components/Picker.svelte';
	import RoleBadge from '#lib/components/RoleBadge.svelte';
	import RoleChoices from '#lib/components/RoleChoices.svelte';
	import type { Role } from '#lib/components/role.ts';
	import RowMenu from '#lib/components/RowMenu.svelte';
	import SearchField from '#lib/components/SearchField.svelte';
	import SecretReveal from '#lib/components/SecretReveal.svelte';
	import Slider from '#lib/components/Slider.svelte';
	import StatusCard from '#lib/components/StatusCard.svelte';
	import Step from '#lib/components/Step.svelte';
	import Steps from '#lib/components/Steps.svelte';
	import PulseDot from '#lib/components/PulseDot.svelte';
	import Select from '#lib/components/Select.svelte';
	import StatusBadge from '#lib/components/StatusBadge.svelte';
	import type { StatusTone } from '#lib/components/status.ts';
	import StatusText from '#lib/components/StatusText.svelte';
	import Table from '#lib/components/Table.svelte';
	import Tabs from '#lib/components/Tabs.svelte';
	import TextField from '#lib/components/TextField.svelte';
	import Switch from '#lib/components/Switch.svelte';
	import ThemeSwitch from '#lib/components/ThemeSwitch.svelte';
	import Time from '#lib/components/Time.svelte';
	import UsageBar from '#lib/components/UsageBar.svelte';
	import type { RowTone, TableColumn } from '#lib/components/table.ts';
	import { icons, type IconName } from '#lib/icons.ts';
	import { deleteConfirmation } from '#lib/projects.ts';
	import Toast from '#lib/components/Toast.svelte';
	import { toastPositions, type ToastPosition } from '#lib/components/toast.ts';
	import { toast, ToastStore } from '#lib/toast.svelte.ts';
	import Grid from './Grid.svelte';
	import Row from './Row.svelte';
	import Section from './Section.svelte';
	import Token from './Token.svelte';

	// Literal class names, so Tailwind builds them. The base tokens first, in the design's order,
	// then the ones derived from them and the states.
	const baseColors = [
		{ token: 'bg', swatch: 'bg-bg' },
		{ token: 'surface', swatch: 'bg-surface' },
		{ token: 'text', swatch: 'bg-text' },
		{ token: 'muted', swatch: 'bg-muted' },
		{ token: 'border', swatch: 'bg-border' },
		{ token: 'accent', swatch: 'bg-accent' },
		{ token: 'error', swatch: 'bg-error' },
		{ token: 'warning', swatch: 'bg-warning' },
		{ token: 'info', swatch: 'bg-info' },
		{ token: 'debug', swatch: 'bg-debug' },
		{ token: 'code-bg', swatch: 'bg-code-bg' },
		{ token: 'selected', swatch: 'bg-selected' }
	];
	const derivedColors = [
		{ token: 'accent-text', swatch: 'bg-accent-text' },
		{ token: 'accent-hover', swatch: 'bg-accent-hover' },
		{ token: 'accent-soft', swatch: 'bg-accent-soft' },
		{ token: 'link-hover', swatch: 'bg-link-hover' },
		{ token: 'hover', swatch: 'bg-hover' },
		{ token: 'focus', swatch: 'bg-focus' },
		{ token: 'current-line', swatch: 'bg-current-line' },
		{ token: 'stripe', swatch: 'bg-stripe' },
		{ token: 'track', swatch: 'bg-track' },
		{ token: 'backdrop', swatch: 'bg-backdrop' },
		{ token: 'error-bg', swatch: 'bg-error-bg' },
		{ token: 'error-badge', swatch: 'bg-error-badge' },
		{ token: 'success', swatch: 'bg-success' },
		{ token: 'success-bg', swatch: 'bg-success-bg' },
		{ token: 'warning-bg', swatch: 'bg-warning-bg' },
		{ token: 'info-bg', swatch: 'bg-info-bg' },
		{ token: 'info-hover', swatch: 'bg-info-hover' },
		{ token: 'debug-bg', swatch: 'bg-debug-bg' },
		{ token: 'row-error', swatch: 'bg-row-error' },
		{ token: 'row-error-even', swatch: 'bg-row-error-even' },
		{ token: 'row-success', swatch: 'bg-row-success' },
		{ token: 'row-success-even', swatch: 'bg-row-success-even' },
		{ token: 'project', swatch: 'bg-project' }
	];

	const iconNames = Object.keys(icons) as IconName[];
	const buttonVariants: ButtonVariant[] = [
		'default',
		'primary',
		'danger',
		'info',
		'neutral',
		'accent',
		'menu',
		'link',
		'muted',
		'title',
		'ghost'
	];
	const badgeTones: Exclude<BadgeTone, 'project'>[] = [
		'error',
		'warning',
		'info',
		'debug',
		'plain'
	];
	const statusTones: StatusTone[] = ['success', 'warning', 'error', 'muted'];

	type SampleProject = {
		id: number;
		name: string;
		dsn: string;
		unresolved: number;
		levels: BadgeTone[];
	};

	const projects: SampleProject[] = [
		{
			id: 1,
			name: 'backend',
			dsn: 'http://0123456789abcdef0123456789abcdef@localhost:8000/1',
			unresolved: 12,
			levels: ['error', 'warning']
		},
		{
			id: 2,
			name: 'frontend',
			dsn: 'http://fedcba9876543210fedcba9876543210@localhost:8000/2',
			unresolved: 0,
			levels: ['error', 'warning', 'info']
		},
		{
			id: 3,
			name: 'worker',
			dsn: 'http://00112233445566778899aabbccddeeff@localhost:8000/3',
			unresolved: 1234,
			levels: []
		}
	];

	type ProjectColumn = TableColumn<'project' | 'dsn' | 'unresolved' | 'levels' | 'actions'>;

	const projectColumns: ProjectColumn[] = [
		{ key: 'project', label: 'Project' },
		{ key: 'dsn', label: 'DSN' },
		{ key: 'unresolved', label: 'Unresolved', number: true },
		{ key: 'levels', label: 'Records', wideOnly: true },
		{ key: 'actions', label: 'Actions', actions: true }
	];

	type Breadcrumb = { time: string; category: string; message: string };
	type TitledColumn = TableColumn<'title' | 'events'>;
	const titledColumns: TitledColumn[] = [
		{ key: 'title', label: 'Issue', fill: true },
		{ key: 'events', label: 'Events', number: true }
	];
	const titledRows = [
		{ id: 1, title: 'TypeError: x is undefined', events: 12 },
		{
			id: 2,
			title:
				"ValueError: login failed {'username': 'root', 'password': '[Filtered]', 'token': '[Filtered]'} while refreshing the session of the checkout service",
			events: 3
		}
	];

	type BreadcrumbColumn = TableColumn<keyof Breadcrumb>;

	const breadcrumbs: Breadcrumb[] = Array.from({ length: 30 }, (_, index) => ({
		time: `16:27:${String(index).padStart(2, '0')}`,
		category: index % 3 === 0 ? 'http' : 'console',
		message: index % 3 === 0 ? `GET /api/orders/${index} 200` : `Loaded order ${index}`
	}));

	const recentBreadcrumbs: Breadcrumb[] = [
		{ time: '16:27:00', category: 'http', message: 'GET /api/orders 200' },
		{ time: '16:27:01', category: 'console', message: 'Failed to load orders' }
	];

	const breadcrumbColumns: BreadcrumbColumn[] = [
		{ key: 'time', label: 'Time' },
		{ key: 'category', label: 'Category' },
		{ key: 'message', label: 'Message' }
	];

	// Samples of the filter components: they change only this page's state.
	let sampleProjects: string[] = $state(['backend']);
	let sampleSearch = $state('');
	let sampleDays = $state({ from: '', to: '' });
	let samplePage = $state(2);
	let samplePerPage = $state(20);
	let sampleDialog = $state(false);
	let sampleNarrowDialog = $state(false);
	let sampleSmallDialog = $state(false);
	const sampleDialogForm = new FormState();
	// SubmitButton: a form that creates something, and an edit form with nothing changed.
	const sampleNewForm = new FormState();
	const sampleSavedForm = new FormState(() => 'saved');

	async function sampleDialogSave() {
		await pause();

		if (sampleName.trim() === 'taken') {
			throw new ApiError(400, 'refused', { fields: { name: 'The name is taken.' } });
		}

		toast.show('Saved.');
	}
	let sampleRole: Role = $state('user');
	let sampleEmail = $state(true);
	let sampleEventLevels: string[] = $state(['fatal', 'error']);
	let sampleLogLevels: string[] = $state([]);
	let sampleName = $state('backend');
	let sampleColor: string = $state(projectPalette[3].color);
	let sampleShare = $state(40);
	let sampleTitle = $state('backend');
	let sampleSwitch = $state(true);
	let sampleTitleValue = $state('backend');
	let sampleTitleError = $state('');
	let sampleStatus = $state('');
	let sampleSelected: ReadonlySet<string | number> = $state(new Set([2]));
	let sampleTab = $state('claude');

	// Samples of actions: they wait a moment, as a request would.
	const pause = () => new Promise((resolve) => setTimeout(resolve, 800));
	const failing = async () => {
		await pause();
		throw new Error('sample failure');
	};

	const sampleMenu: MenuItem[] = [
		{ label: 'Rename', icon: 'edit', action: () => toast.show('Rename chosen.') },
		{ label: 'Show installation code', icon: 'code', href: '#row-menu' },
		{ label: 'Settings', icon: 'settings', href: '#row-menu', separatorBefore: true },
		{ label: 'Logs', icon: 'logs', href: '#row-menu' },
		{
			label: 'Delete',
			icon: 'delete',
			danger: true,
			separatorBefore: true,
			confirm: {
				title: 'Delete project',
				message: 'Delete the project worker with all its issues, events and logs?',
				confirmLabel: 'Delete'
			},
			action: pause
		},
		{
			label: 'Delete (from env)',
			icon: 'delete',
			separatorBefore: true,
			disabled: 'Remove it from PROJECTS first.'
		}
	];

	// The row menu of the design: its actions, the destructive one last after a line.
	const rowActions: MenuItem[] = [
		{ label: 'Edit', icon: 'edit', href: '#row-menu' },
		{ label: 'Show installation code', icon: 'code', href: '#row-menu' },
		{
			label: 'Delete',
			icon: 'delete',
			danger: true,
			separatorBefore: true,
			confirm: {
				title: 'Delete connection',
				message: 'Delete the connection Team chat? Nothing more is sent to it.',
				confirmLabel: 'Delete'
			},
			action: pause
		}
	];

	const sampleProjectDelete = deleteConfirmation('worker');

	const sampleConfirm: Confirmation = {
		title: 'Delete user',
		message:
			'Delete the user developer? They are logged out and their coding agents are disconnected.',
		confirmLabel: 'Delete'
	};
	const sampleReset: Confirmation = {
		title: 'Reset password',
		message: 'Give developer a new temporary password? Their coding agents are disconnected.',
		confirmLabel: 'Reset',
		tone: 'primary'
	};

	type SampleFact = { label: string; value: string };

	const sampleFacts: SampleFact[] = [
		{ label: 'Events', value: '1,204' },
		{ label: 'First seen', value: '3 days ago' },
		{ label: 'Release', value: 'backend@1.4.2' },
		{ label: 'Environment', value: 'production' }
	];

	type EventFact = { label: string; value: string; kind?: 'time' | 'code' };

	const sampleEventFacts: EventFact[] = [
		{ label: 'Received', value: '2026-11-20T10:00:00Z', kind: 'time' },
		{ label: 'Release', value: 'web@1.4.2', kind: 'code' },
		{ label: 'Environment', value: 'production' }
	];

	const sampleKept: SampleFact[] = [
		{ label: 'Logs below error', value: 'since 20 Nov · 3 days' },
		{ label: 'Error logs', value: 'none stored' },
		{ label: 'Errors', value: 'since 23 Nov · today' }
	];

	const statusOptions = [
		{ value: 'unresolved', label: 'Unresolved' },
		{ value: 'resolved', label: 'Resolved' },
		{ value: 'ignored', label: 'Ignored' }
	];

	// Toasts: a store and a Toast of the page's own, so the position picker leaves the app's alone.
	const sampleToasts = new ToastStore();
	let sampleToastPosition: ToastPosition = $state('bottom-right');
	const sampleToastPositions = Object.keys(toastPositions).map((value) => ({
		value,
		label: value.replace('-', ' ')
	}));

	function sampleToastStack() {
		sampleToasts.show('Issue resolved.');
		sampleToasts.show('Could not reach Telegram.', 'error');
		sampleToasts.show('The code was sent again.', 'info');
	}

	const sampleTabs = [
		{ id: 'claude', label: 'Claude Code' },
		{ id: 'codex', label: 'Codex' },
		{ id: 'other', label: 'Other' }
	];

	const sortedColumns: ProjectColumn[] = [
		{
			key: 'project',
			label: 'Project',
			sort: { ascending: 'project', descending: 'project_desc', first: 'ascending' }
		},
		{ key: 'dsn', label: 'DSN' },
		{
			key: 'unresolved',
			label: 'Unresolved',
			number: true,
			sort: { ascending: 'unresolved_asc', descending: 'unresolved', first: 'descending' }
		},
		{ key: 'levels', label: 'Records', wideOnly: true },
		{ key: 'actions', label: 'Actions', actions: true }
	];

	const sampleTone = (row: SampleProject): RowTone | undefined =>
		row.unresolved > 100 ? 'error' : row.unresolved === 0 ? 'success' : 'faded';

	type Context = { name: string; value: string };
	type ContextColumn = TableColumn<keyof Context>;

	const contextColumns: ContextColumn[] = [
		{ key: 'name', label: 'Name', rowHeader: true },
		{ key: 'value', label: 'Value' }
	];
	const contexts: Context[] = [
		{ name: 'runtime', value: 'node v24.11.0' },
		{ name: 'os', value: 'Linux 6.8' },
		{ name: 'browser', value: 'Firefox 140' }
	];

	const sampleJson = '{\n  "app_memory": 264241152,\n  "device": "x86_64"\n}';
	const sampleSnippet =
		'Sentry.init({\n  dsn: "http://key@localhost:8000/1",\n  enableLogs: true\n});';

	const sampleLog =
		'{"level":"error","msg":"Payment failed","err":"Error: card declined\\n    at charge (pay.js:12)\\n    at handler (api.js:40)"}';
</script>

<div id="exceptic-design-system" class="max-w-[980px]">
	<PageHeading title="Design system" />
	<p class="max-w-[70ch] text-muted">
		The components the screens use, in one place. Colors are tokens in <code>layout.css</code>
		and follow the light or dark system theme. Only the dev server serves this page.
	</p>

	<Section title="Color tokens" id="colors">
		{#snippet lead()}Base tokens, redefined for dark mode in <code>layout.css</code>.{/snippet}
		<div class="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-2.5">
			{#each baseColors as color (color.token)}
				<Token swatch={color.swatch} value="--color-{color.token}">{color.token}</Token>
			{/each}
		</div>
		<Row caption="Derived from the base tokens, and the states">
			<div class="grid w-full grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-2.5">
				{#each derivedColors as color (color.token)}
					<Token swatch={color.swatch} value="--color-{color.token}">{color.token}</Token>
				{/each}
			</div>
		</Row>
	</Section>

	<Section title="Project palette" id="palette">
		{#snippet lead()}
			Ten presets, readable as badge text in light and dark. A new project gets the first preset no
			other project shows yet; one without a color of its own takes one by its ID.
		{/snippet}
		<div class="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-2.5">
			{#each projectPalette as entry (entry.color)}
				<Token color={entry.color} value={entry.color}>
					<Badge tone="project" label={entry.name.toLowerCase()} color={entry.color} />
				</Token>
			{/each}
		</div>
	</Section>

	<Section title="Icons" id="icons">
		{#snippet lead()}
			<Button variant="link" href="https://lucide.dev" external>Lucide</Button> icons, drawn in the text
			color with a 2px stroke. Default size 18px; the cards show them at 24px, named by their key in
			<code>icons.ts</code>.
		{/snippet}
		<div class="grid grid-cols-[repeat(auto-fill,minmax(130px,1fr))] gap-2">
			{#each iconNames as name (name)}
				<div
					class="flex flex-col items-center gap-2 rounded-popup border border-border bg-surface px-2 py-3.5"
				>
					<Icon {name} size={24} />
					<code class="text-2xs text-muted">{name}</code>
				</div>
			{/each}
		</div>
	</Section>

	<Section title="Buttons" id="buttons">
		{#snippet note()}
			Every boxed button has the same hover: the border takes the button's tone and a soft ring
			glows around it. There is no press offset.
		{/snippet}
		<Grid>
			<Card>
				<Row caption="variant default / primary / danger">
					<Button>Cancel</Button>
					<Button variant="primary">Save</Button>
					<Button variant="danger">Delete</Button>
				</Row>
			</Card>
			<Card>
				<Row caption="variant accent with href / link / disabled">
					<Button variant="accent" href="#buttons">Open</Button>
					<Button variant="link">Show more</Button>
					<Button variant="primary" disabled>Disabled</Button>
				</Row>
			</Card>
			<Card>
				<Row caption="size large: the one main action of a page">
					<Button variant="primary" size="large">Add project</Button>
				</Row>
			</Card>
			<Card>
				<Row caption="CopyButton with a label, info / neutral (MCP commands) / AgentCopyButton">
					<CopyButton text="Copied text" title="Copy the sample" label="Copy" />
					<CopyButton text="Copied text" title="Copy the command" label="Copy" variant="neutral" />
					<AgentCopyButton request={'Exceptic MCP: get_issue {"issue_id":"k3x9q2mf"}'} />
				</Row>
			</Card>
		</Grid>
		{#each buttonVariants as variant (variant)}
			<Row caption={`variant="${variant}"`}>
				<Button {variant}>Text</Button>
				<Button {variant} icon="edit">With icon</Button>
				<Button {variant} icon={variant === 'menu' ? 'more' : 'delete'} title="Icon only" />
				<Button {variant} href="#buttons">Link</Button>
				<Button {variant} disabled>Disabled</Button>
			</Row>
		{/each}
		<Row caption="busy: its action runs (clicks ignored, the focus stays)">
			<Button variant="primary" busy>Saving</Button>
			<Button variant="danger" busy>Deleting</Button>
		</Row>
		<Row
			caption="SubmitButton: a form's submit through its FormState; an edit form with nothing changed does nothing and says why"
		>
			<SubmitButton form={sampleNewForm}>Add</SubmitButton>
			<SubmitButton form={sampleSavedForm} />
		</Row>
		<Row caption="external: another site in a new tab, without referrer or opener">
			<Button variant="info" icon="externalLink" href="https://example.com" external>
				Find chat ID
			</Button>
			<Button variant="info" icon="externalLink" disabled>Find chat ID</Button>
			<Button variant="accent" icon="view" href="#buttons" title="Open the issue" />
		</Row>
		<Row caption="BackButton: back to the page the viewer came from, or to the page above">
			<BackButton fallback="#buttons" />
		</Row>
	</Section>

	<Section title="Fields" id="fields">
		<Grid>
			<Card>
				<TextField
					label="Project name"
					name="name"
					placeholder="backend"
					bind:value={sampleName}
					hint="Lowercase letters, digits, - and _."
				/>
				<TextField label="Email" name="email" type="email" placeholder="you@example.com" />
				<TextField
					label="Refused name"
					name="refused"
					value="Back End"
					error="Use lowercase letters, digits, - and _."
				/>
				<Row caption="Select field: its label above it, as wide as the other fields">
					<Select
						field
						label="Status"
						all="All statuses"
						value={sampleStatus}
						options={statusOptions}
						onchange={(value: string) => (sampleStatus = value)}
					/>
				</Row>
				<TextField label="Name" name="display_name" note="optional: the username when empty" />
				<TextField label="Username" name="username" value="developer" readonly />
				<TextField label="Bot token" name="bot_token" type="password" saved="1234…wxyz" />
				<TextField
					label="Connection name"
					name="connection_name"
					description="Optional: left empty, it is filled in when saved."
				/>
				<TextField
					label="Confirmation code"
					name="code"
					variant="code"
					placeholder="123456"
					autocomplete="one-time-code"
				>
					{#snippet action()}<Button variant="primary">Confirm</Button>{/snippet}
				</TextField>
				<TextField label="Email address" name="address" type="email" placeholder="team@example.com">
					{#snippet action()}<Button variant="primary">Verify</Button>{/snippet}
				</TextField>
			</Card>
			<Card>
				<Row
					caption="variant hero: the one large field of a page, its label for screen readers"
					wide
				>
					<TextField
						label="Project name"
						name="hero_name"
						placeholder="Project name, e.g. backend"
						variant="hero"
					/>
				</Row>
				<Row caption="Switch: on, off and disabled (role switch)">
					<Switch label="Send notifications" bind:checked={sampleSwitch} />
					<Switch label="Off" checked={false} />
					<Switch label="Disabled" checked disabled />
				</Row>
				<Row caption="Checkbox and radio: one look everywhere" wide>
					<LevelChoices
						kind="event"
						legend="Error levels"
						selected={sampleEventLevels}
						onchange={(chosen: string[]) => (sampleEventLevels = chosen)}
					/>
				</Row>
				<ChoiceGroup legend="Send by" inline>
					<Choice
						type="radio"
						label="Email"
						checked={sampleEmail}
						onchange={() => (sampleEmail = true)}
					/>
					<Choice
						type="radio"
						label="Slack"
						checked={!sampleEmail}
						onchange={() => (sampleEmail = false)}
					/>
				</ChoiceGroup>
				<ChoiceGroup legend="Send to" inline error="Choose where to send.">
					<Choice
						type="checkbox"
						label="Email"
						checked={sampleEmail}
						onchange={(checked: boolean) => (sampleEmail = checked)}
					/>
					<Choice
						type="checkbox"
						label="Slack"
						checked={false}
						onchange={() => {}}
						disabled
						reason="set SECRET_KEY"
					/>
				</ChoiceGroup>
				<RoleChoices bind:value={sampleRole} />
			</Card>
			<Card>
				<ColorField bind:value={sampleColor} name="backend" hint={help.fields.Projects.Color} />
				<ColorField
					value="#123456"
					name="custom"
					error="A color of its own: the rainbow swatch shows it."
				/>
			</Card>
			<Card>
				<Slider
					label="Slider: a scale with marks under the thumb and the value beside it"
					bind:value={sampleShare}
					step={5}
					least={5}
					marks={[0, 50, 100]}
					unit="%"
					hint="The thumb stops at 5."
				>
					{#snippet output()}<b>{sampleShare}%</b>
						<span class="text-muted">· up to 819.2 MB</span>{/snippet}
				</Slider>
				<Slider label="Refused" value={50} error="Choose 5 to 100 in steps of 5." />
			</Card>
			<Card>
				<ChoiceGroup legend="Projects (lead, hidden legend)" hideLegend inline>
					{#snippet lead()}
						<Choice
							type="checkbox"
							label="All projects, including ones added later"
							checked={false}
							onchange={() => {}}
						/>
					{/snippet}
					<Choice type="checkbox" label="backend" checked onchange={() => {}} />
					<Choice type="checkbox" label="frontend" checked={false} onchange={() => {}} />
				</ChoiceGroup>
				<ChoiceGroup legend="Options (hints beside)" hideLegend inline>
					<Choice
						type="checkbox"
						label="Active: send notifications"
						hint={help.fields.Notifications.Active}
						checked={sampleEmail}
						onchange={(checked: boolean) => (sampleEmail = checked)}
					/>
				</ChoiceGroup>
				<LevelChoices
					kind="log"
					legend="Log levels"
					beside
					selected={sampleLogLevels}
					onchange={(chosen: string[]) => (sampleLogLevels = chosen)}
				/>
				<LevelChoices
					kind="event"
					legend="Error levels in a column"
					inline={false}
					selected={sampleEventLevels}
					onchange={(chosen: string[]) => (sampleEventLevels = chosen)}
				/>
				<Row caption="LevelSummary: the chosen levels, highest first, or none">
					<LevelSummary kind="event" chosen={sampleEventLevels} />
					<LevelSummary kind="log" chosen={sampleLogLevels} />
				</Row>
			</Card>
		</Grid>
	</Section>

	<Section title="Row actions" id="row-menu">
		{#snippet lead()}
			One ⋯ button at the end of a table row opens the row's actions. Destructive ones come last, in
			red, after a line.
		{/snippet}
		<Card>
			<div class="flex flex-wrap items-center justify-between gap-2.5">
				<span><b>Team chat</b> <span class="text-muted">Slack</span></span>
				<StatusText tone="success">Active</StatusText>
				<RowMenu label="Actions for Team chat" items={rowActions} />
			</div>
		</Card>
		<Row
			caption="RowMenu items as data: links, actions, a confirm asked first, a disabled one says why"
		>
			<span>worker</span>
			<RowMenu label="Actions for worker" items={sampleMenu} />
		</Row>
	</Section>

	<Section title="Badges and states" id="badges">
		<Row>
			{#each levels.event as level (level)}
				<LevelBadge {level} />
			{/each}
			<Badge tone="project" label="backend" />
			<Badge tone="project" label="frontend" />
			{#each ['unresolved', 'resolved', 'ignored'] as status (status)}
				<StatusBadge {status} />
			{/each}
			<StatusText tone="success">Active</StatusText>
			<StatusText tone="muted">Paused</StatusText>
		</Row>
		<Row caption="Badge tone">
			{#each badgeTones as tone (tone)}
				<Badge {tone} label={tone} />
			{/each}
		</Row>
		<Row caption="project tone (its color from the session), as a link">
			<Badge tone="project" label="backend" />
			<Badge tone="project" label="frontend" />
			<Badge tone="project" label="worker" href="#badges" title="Issues of worker" />
			<Badge tone="error" label="error" href="#badges" />
		</Row>
		<Row
			caption="StatusBadge: an issue status, its sign and name in its tone (its meaning in the tooltip)"
		>
			{#each ['unresolved', 'resolved', 'ignored', 'all'] as status (status)}
				<StatusBadge {status} />
			{/each}
			<StatusBadge status="resolved" href="#badges" />
		</Row>
		<Row caption="project tone, size heading: beside a page's title">
			<Badge tone="project" label="backend" size="heading" />
		</Row>
		<Row caption="project tone with color: a color not saved yet (the project form's preview)">
			<Badge tone="project" label="mobile" color="#0d9488" />
		</Row>
		<Row caption="RoleBadge: the roles an administrator gives plain, the superadmin standing out">
			<RoleBadge role="superadmin" />
			<RoleBadge role="admin" />
			<RoleBadge role="user" />
		</Row>
		<Row caption="StatusText: a state as a pill with a dot">
			{#each statusTones as tone (tone)}
				<StatusText {tone}>{tone}</StatusText>
			{/each}
		</Row>
		<Row caption="LevelBadge: event levels, in the color group the server gives them">
			{#each levels.event as level (level)}
				<LevelBadge {level} />
			{/each}
		</Row>
		<Row caption="LevelBadge: log levels">
			{#each levels.log as level (level)}
				<LevelBadge {level} />
			{/each}
		</Row>
		<Row caption="Time: the distance, the exact time in the tooltip">
			<Time at="2026-11-20T10:00:00Z" />
		</Row>
	</Section>

	<Section title="Tooltips" id="tooltips">
		{#snippet lead()}
			Tooltips are the browser's own, from <code>title</code>: an icon-only button gets its name as
			the tooltip and its label, and a help hint is a Hint (?) that takes the focus and gives its
			text as its name.
		{/snippet}
		{#snippet note()}
			Write <code>title</code> on a Button or a Hint's text; never a tooltip by hand.
		{/snippet}
		<Card>
			<div class="flex flex-wrap items-center gap-2.5">
				<Button title="Opens the issue in a new tab">Hover me</Button>
				<Button variant="menu" icon="copy" title="Copy" />
				<span class="inline-flex items-center gap-1.25">
					Help hint <Hint text={help.fields.Notifications.Active} />
				</span>
				<span class="inline-flex items-center gap-1.25">
					Long hint <Hint text={help.fields.Projects['Disk share']} />
				</span>
			</div>
		</Card>
	</Section>

	<Section title="Cards and sections" id="cards">
		{#snippet lead()}
			Every box around a group of content is a card: no visible border, a soft shadow, 16px corners,
			roomy padding. Tables are cards too; a table in a scroll box lies flat.
		{/snippet}
		<Grid>
			<Card title="Storage">
				<p class="text-xs text-muted">All projects</p>
				<p>Database: <b>1.2 GB</b> of 2.0 GB</p>
				<UsageBar percent={60} label="Database size against its limit" />
			</Card>
			<ChoiceGroup legend="Type" card>
				<Choice
					type="radio"
					label="Email"
					checked={sampleEmail}
					onchange={() => (sampleEmail = true)}
				/>
				<Choice
					type="radio"
					label="Slack"
					checked={!sampleEmail}
					onchange={() => (sampleEmail = false)}
				/>
			</ChoiceGroup>
		</Grid>
	</Section>

	<Section title="Expandable sections" id="details">
		{#snippet lead()}
			A card whose header ends in a round chevron button; the chevron turns up when open. The count
			sits next to the title as a pill, a long detail is cut to one line. Small disclosures inside a
			section (contexts, stack frames) use a small chevron that turns down.
		{/snippet}
		<Details title="Breadcrumbs" count={recentBreadcrumbs.length} open>
			<Table
				columns={breadcrumbColumns}
				rows={recentBreadcrumbs}
				rowKey={(row: Breadcrumb) => row.time}
				label="Breadcrumbs"
				compact
				scroll
			>
				{#snippet cell(row: Breadcrumb, column: BreadcrumbColumn)}{row[column.key]}{/snippet}
			</Table>
		</Details>
		<Details title="Request">
			{#snippet summary()}<code>GET http://localhost:3500/api/debug-sentry/cause</code>{/snippet}
			<p>A detail cut to one line in the heading.</p>
		</Details>
		<Details title="Levels to record" variant="form" hint="What this project stores.">
			{#snippet summary()}<LevelSummary kind="log" chosen={sampleLogLevels} />{/snippet}
			<LevelChoices
				kind="log"
				legend="Logs"
				beside
				selected={sampleLogLevels}
				onchange={(chosen: string[]) => (sampleLogLevels = chosen)}
			/>
		</Details>
		<Details title="Projects" variant="form" invalid>
			{#snippet summary()}<span class="text-muted">none</span>{/snippet}
			<p class="text-error">Choose at least one project (invalid: it opens).</p>
		</Details>
		<Details title="Disk share" variant="form" open>
			{#snippet summary()}<span class="text-muted">up to 40% · 819.2 MB</span>{/snippet}
			<p>open: a form section that starts open.</p>
		</Details>
		<Details title="Advanced settings" variant="disclosure">
			{#snippet summary()}Color, logs to record, disk share. Defaults work for most projects.{/snippet}
			<FormGroup title="Color" hint={help.fields.Projects.Color}>
				<p>disclosure: optional settings, parted by FormGroup.</p>
			</FormGroup>
			<FormGroup title="Logs to record">
				<p>A dashed line above every part but the first.</p>
			</FormGroup>
		</Details>
		<Details title="app" variant="inline">
			<CodeBlock code={sampleJson} />
		</Details>
	</Section>

	<Section title="Breadcrumbs" id="breadcrumbs">
		{#snippet lead()}
			The path above a page title, ancestors muted, split by slashes. The parent link takes Esc.
		{/snippet}
		<Card>
			<Breadcrumbs
				items={[
					{ label: 'Projects', href: '#breadcrumbs' },
					{ label: 'backend', href: '#breadcrumbs-backend' },
					{ label: 'Installation', href: '#breadcrumbs-setup' }
				]}
			/>
			<PageHeading title="Connect backend" tabTitle={false} />
		</Card>
	</Section>

	<Section title="Added for the web app" id="web-app-components">
		{#snippet lead()}
			Components the web app adds to the ones above, each built once and used by every page that
			needs it.
		{/snippet}
		<Row
			caption="ConfirmDialog: every delete, reset and bulk action asks here; Cancel has the focus first, Esc and a click beside it cancel, the action shows its progress and a refusal stays inside"
		>
			<ConfirmButton confirm={sampleProjectDelete} action={pause}>Delete</ConfirmButton>
			<ConfirmButton
				confirm={sampleConfirm}
				action={failing}
				icon="delete"
				title="Delete, and fail"
			/>
			<ConfirmButton variant="default" confirm={sampleReset} action={pause}>
				Reset password
			</ConfirmButton>
			<ConfirmButton confirm={sampleConfirm} action={pause} disabled title="Nothing to delete">
				Delete (disabled)
			</ConfirmButton>
		</Row>
		<Row caption="Notices: info, success, warning, error; compact beside a form's fields" wide>
			<div class="flex flex-col gap-2.5">
				<Notice tone="info">Nothing is deleted: the limit is off.</Notice>
				<Notice tone="success">Project and its data deleted.</Notice>
				<Notice tone="warning">
					This server is reached over plain HTTP, so sign-in codes and tokens cross the network
					unencrypted.
				</Notice>
				<Notice tone="error">Could not delete the project: it comes from PROJECTS.</Notice>
				<Notice tone="info" compact>
					<p>The share matters only once the database nears its limit.</p>
					<p>100% (the default) means no limit of its own.</p>
				</Notice>
			</div>
		</Row>
		<Row caption="Active filters: a chip per field, each value removable, then Clear all" wide>
			<ActiveFilters
				chips={[
					{
						label: 'Project',
						values: sampleProjects.map((name) => ({
							text: name,
							kind: 'project' as const,
							remove: '#filters'
						}))
					},
					{
						label: 'Level',
						values: [{ text: 'error', kind: 'level' as const, remove: '#filters' }]
					},
					{ label: 'Search', values: [{ text: 'timeout', remove: '#filters' }] }
				]}
				clear="#filters"
			/>
		</Row>
		<Row
			caption="Pager: the rows shown, previous and next, the pages around the current one, the page size"
			wide
		>
			<Pager
				page={samplePage}
				perPage={samplePerPage}
				total={134}
				href={() => '#web-app-components'}
				onperpage={(perPage: number) => ((samplePerPage = perPage), (samplePage = 1))}
			/>
		</Row>
		<Row
			caption="Usage bar: the database size, a segment per project in its color and the rest"
			wide
		>
			<UsageBar
				segments={[
					{ label: 'backend: 1.1 GB', percent: 55, color: projectPalette[0].color },
					{ label: 'frontend: 420 MB', percent: 21, color: projectPalette[1].color },
					{ label: 'Other: 60 MB', percent: 3 }
				]}
				label="backend 1.1 GB, frontend 420 MB, other 60 MB of 2.0 GB"
			/>
		</Row>
		<Row caption="Secret shown once: a temporary password or a personal token, copied whole" wide>
			<SecretReveal
				value="kT9q-2mFx-Wp7z"
				label="Temporary password"
				copyTitle="Copy the temporary password"
				copied="Temporary password copied to clipboard."
			>
				<b>Temporary password for lead</b>: shown once, copy it now.
			</SecretReveal>
		</Row>
		<Row caption="Log line: cut with Expand when long, Full screen with JSON fields as lines" wide>
			<LogBody text={sampleLog} />
		</Row>
		<Row caption="Facts: label and value pairs (an event, a user)" wide>
			<Facts facts={sampleEventFacts}>
				{#snippet value(fact: EventFact)}
					{#if fact.kind === 'time'}<Time at={fact.value} />{:else if fact.kind === 'code'}<code>
							{fact.value}
						</code>{:else}{fact.value}{/if}
				{/snippet}
			</Facts>
		</Row>
		<Row caption="Facts boxed: cells of a framed grid" wide>
			<Facts facts={sampleFacts} boxed>
				{#snippet value(fact: SampleFact)}<b>{fact.value}</b>{/snippet}
			</Facts>
		</Row>
		<Row caption="Facts with a title and hint" wide>
			<Facts
				facts={sampleKept}
				boxed
				title="History kept"
				hint={help.fields.Projects['History kept']}
			>
				{#snippet value(fact: SampleFact)}<b>{fact.value}</b>{/snippet}
			</Facts>
		</Row>
		<Row caption="Status words: sent, waiting, failed (the delivery log, cleanup runs)">
			<StatusText tone="success">sent</StatusText>
			<StatusText tone="warning">waiting</StatusText>
			<StatusText tone="error">failed</StatusText>
		</Row>
	</Section>

	<Section title="Copy, code and secrets" id="code">
		<Row caption="CopyButton icon only: copies, then says so in the app's toast">
			<CopyButton text="http://key@localhost:8000/1" title="Copy DSN" copied="DSN copied." />
		</Row>
		<Row caption="CopyField: a read-only value with its copy button" wide>
			<CopyField
				value="http://0123456789abcdef@localhost:8000/1"
				label="DSN of backend"
				copyTitle="Copy the DSN of backend"
				copied="DSN copied."
			/>
		</Row>
		<Row caption="CodeBlock plain: text as given, long lines wrapped (indented JSON)" wide>
			<CodeBlock code={sampleJson} label="Context" />
		</Row>
		<Row caption="CodeBlock command: one line after ›, with its copy button" wide>
			<CodeBlock
				variant="command"
				code="claude mcp add --transport http exceptic http://localhost:8000/mcp"
				label="Claude Code command"
				copyTitle="Copy the command"
				copied="Command copied to clipboard."
			/>
		</Row>
		<Row
			caption="CodeBlock command, copyVariant neutral: the grey copy button of the MCP page"
			wide
		>
			<CodeBlock
				variant="command"
				code="codex mcp add exceptic --url http://localhost:8000/mcp"
				label="Codex command"
				copyTitle="Copy the command"
				copyVariant="neutral"
			/>
		</Row>
		<Row caption="CodeBlock snippet: several lines to paste, with its copy button" wide>
			<CodeBlock
				variant="snippet"
				code={sampleSnippet}
				label="SDK setup"
				copyTitle="Copy the code"
			/>
		</Row>
		<Row caption="Steps and Step: numbered steps, the content indented to the text" wide>
			<Steps>
				<Step title="Install the SDK">
					<CodeBlock
						variant="snippet"
						code="npm install @sentry/node"
						label="Install command"
						copyTitle="Copy the install command"
					/>
				</Step>
				<Step title="Start it with your app" note="As early as possible, before the rest loads.">
					<CodeBlock variant="snippet" code={sampleSnippet} label="Setup code" copyTitle="Copy" />
				</Step>
				<Step title="Send a test error" optional>
					<CodeBlock
						variant="snippet"
						code="Sentry.captureException(new Error('Test error'));"
						label="Test line"
						copyTitle="Copy the test line"
					/>
				</Step>
			</Steps>
		</Row>
		<Row caption="CopyField masked: dots until Show; the button copies the value" wide>
			<CopyField
				value="exc_0123456789abcdef0123456789abcdef"
				label="Personal token"
				copyTitle="Copy the token"
				masked
			/>
		</Row>
	</Section>

	<Section title="Watched states" id="status-cards">
		<Row caption="PulseDot: still waiting (no pulse under reduced motion)">
			<PulseDot />
		</Row>
		<Row caption="StatusCard waiting: a PulseDot, the state, a line and actions" wide>
			<StatusCard state="waiting" title="Waiting for backend to connect…">
				Most SDKs check in as soon as the app starts. This page updates by itself.
				{#snippet actions()}<Button href="#status-cards">Do it later</Button>{/snippet}
			</StatusCard>
		</Row>
		<Row caption="StatusCard done: a check in a soft green circle" wide>
			<StatusCard state="done" title="backend is connected">
				Its errors appear on Issues and its logs on Logs as they arrive.
				{#snippet actions()}<Button href="#status-cards">Open issues</Button>{/snippet}
			</StatusCard>
		</Row>
	</Section>

	<Section title="Toasts" id="toasts">
		<Row caption="Toast: success, error, info; the time bar pauses under the pointer or focus">
			<Button onclick={() => sampleToasts.show('Saved.')}>Success</Button>
			<Button onclick={() => sampleToasts.show('Could not copy.', 'error')}>Error</Button>
			<Button onclick={() => sampleToasts.show('The code was sent again.', 'info')}>Info</Button>
			<Button onclick={sampleToastStack}>Three at once</Button>
		</Row>
		<Row caption="position: the corner of the stack (the app's stays bottom right)">
			<Select
				label="Toast position"
				value={sampleToastPosition}
				options={sampleToastPositions}
				onchange={(value: string) => (sampleToastPosition = value as ToastPosition)}
			/>
		</Row>
		<Toast toasts={sampleToasts} position={sampleToastPosition} />
	</Section>

	<Section title="Filters" id="filters">
		<Row
			caption="FilterBar with a Picker (option snippet: badges), SearchField and DateRange (a month calendar: the first day, then the last)"
			wide
		>
			<FilterBar label="Sample filters">
				<Picker
					label="Projects"
					all="All projects"
					options={['backend', 'frontend']}
					selected={sampleProjects}
					onchange={(selected: string[]) => (sampleProjects = selected)}
				>
					{#snippet option(name: string)}<Badge tone="project" label={name} />{/snippet}
				</Picker>
				<SearchField
					value={sampleSearch}
					label="Search text"
					onchange={(value: string) => (sampleSearch = value)}
				/>
				<DateRange
					from={sampleDays.from}
					to={sampleDays.to}
					onchange={(from: string, to: string) => (sampleDays = { from, to })}
				/>
				<Select
					label="Status"
					all="All statuses"
					value={sampleStatus}
					options={statusOptions}
					onchange={(value: string) => (sampleStatus = value)}
				/>
			</FilterBar>
		</Row>
	</Section>

	<Section title="Tabs" id="tabs">
		<Row caption="Tabs: the arrows, Home and End move; param puts the tab in the address" wide>
			<Tabs label="MCP client" tabs={sampleTabs} bind:selected={sampleTab}>
				{#snippet panel(id: string)}<p class="text-muted">The panel of {id}.</p>{/snippet}
			</Tabs>
		</Row>
		<Row caption="variant underline: names over a line, the chosen one underlined" wide>
			<Tabs label="MCP client (underline)" tabs={sampleTabs} variant="underline">
				{#snippet panel(id: string)}<p class="text-muted">The panel of {id}.</p>{/snippet}
			</Tabs>
		</Row>
	</Section>

	<Section title="Dialogs and confirmations" id="dialogs">
		<Row caption="Dialog wide: a bar with its title and Close, the content scrolls inside">
			<Button onclick={() => (sampleDialog = true)}>Open a dialog</Button>
			<Dialog title="A dialog" bind:open={sampleDialog}><p>Its content scrolls inside.</p></Dialog>
		</Row>
		<Row
			caption="Dialog narrow: its title, the content and the actions; Esc or a click beside it close it"
		>
			<Button onclick={() => (sampleSmallDialog = true)}>Open a narrow dialog</Button>
			<Dialog title="Leave the page?" size="narrow" bind:open={sampleSmallDialog}>
				<p>What you typed is not saved.</p>
				{#snippet actions()}
					<Button onclick={() => (sampleSmallDialog = false)}>Cancel</Button>
					<Button variant="primary" onclick={() => (sampleSmallDialog = false)}>Leave</Button>
				{/snippet}
			</Dialog>
		</Row>
		<Row caption="FormDialog: a short form in a narrow Dialog; a refused field keeps it open">
			<Button onclick={() => (sampleNarrowDialog = true)}>Rename</Button>
			<FormDialog
				title="Rename project"
				bind:open={sampleNarrowDialog}
				form={sampleDialogForm}
				save={sampleDialogSave}
			>
				<TextField
					label="Name"
					name="name"
					bind:value={sampleName}
					error={sampleDialogForm.fields.name}
					note="try: taken"
				/>
			</FormDialog>
		</Row>
		<Row
			caption="IssueActions: an issue's page (by its status), and the checked issues of a list (off with none checked)"
		>
			<IssueActions issueIds={['sample00']} status="unresolved" canDelete ondone={pause} />
			<IssueActions issueIds={['sample00']} status="resolved" canDelete={false} ondone={pause} />
			<IssueActions issueIds={[]} canDelete ondone={pause} />
		</Row>
		<Row caption="BulkBar: above a list with checkboxes, the count checked or why not yet" wide>
			<BulkBar count={3}>
				<ConfirmButton
					confirm={{
						title: 'Delete logs',
						message: 'Delete 3 selected logs?',
						confirmLabel: 'Delete'
					}}
					action={pause}
				>
					Delete
				</ConfirmButton>
			</BulkBar>
			<BulkBar count={0} problem="Select logs to delete them">
				<ConfirmButton
					confirm={{ title: 'Delete logs', message: '', confirmLabel: 'Delete' }}
					action={pause}
					disabled
				>
					Delete
				</ConfirmButton>
			</BulkBar>
		</Row>
	</Section>

	<Section title="Menu" id="menu">
		<Row
			caption="NavLink: an item, the current page, with a count; the Sidebar of this page uses them"
		>
			<div
				class="flex w-[236px] flex-col gap-0.5 rounded-popup border border-border bg-surface p-2"
			>
				<NavLink label="Logs" icon="logs" href="#menu" />
				<NavLink label="Projects" icon="projects" href="#menu" active />
				<NavLink
					label="Issues"
					icon="issues"
					href="#menu"
					count={1234}
					countLabel="1234 unresolved"
				/>
			</div>
		</Row>
		<Row caption="NavLink collapsed: icons only, the count on the corner">
			<div class="flex w-[62px] flex-col gap-0.5 rounded-popup border border-border bg-surface p-2">
				<NavLink
					label="Issues"
					icon="issues"
					href="#menu"
					count={7}
					countLabel="7 unresolved"
					collapsed
				/>
				<NavLink label="Logs" icon="logs" href="#menu" collapsed />
			</div>
		</Row>
		<Row
			caption="ThemeSwitch: Dark mode, the system's theme until toggled, then light or dark kept in this browser (it changes this page too)"
		>
			<div
				class="flex w-[236px] flex-col gap-0.5 rounded-popup border border-border bg-surface p-2"
			>
				<ThemeSwitch />
			</div>
			<div class="flex w-[62px] flex-col gap-0.5 rounded-popup border border-border bg-surface p-2">
				<ThemeSwitch collapsed />
			</div>
		</Row>
	</Section>

	<Section title="Tables" id="tables">
		<Row caption="number, wideOnly (hidden under 701px) and actions columns" wide>
			<Table
				columns={projectColumns}
				rows={projects}
				rowKey={(row: SampleProject) => row.id}
				label="Projects"
			>
				{#snippet cell(row: SampleProject, column: ProjectColumn)}
					{#if column.key === 'project'}
						<Badge tone="project" label={row.name} /> <span class="text-muted">#{row.id}</span>
					{:else if column.key === 'dsn'}
						<span class="flex items-center gap-2">
							<code class="break-all">{row.dsn}</code>
							<CopyButton text={row.dsn} title="Copy DSN" copied="DSN copied." />
						</span>
					{:else if column.key === 'unresolved'}
						<a href="#tables">{row.unresolved}</a>
					{:else if column.key === 'levels'}
						<span class="inline-flex flex-wrap gap-1">
							{#each row.levels as level (level)}
								<Badge tone={level} label={level} />
							{:else}
								<span class="text-muted">none</span>
							{/each}
						</span>
					{:else}
						<Button icon="edit" title={`Edit ${row.name}`} href="#tables" />
					{/if}
				{/snippet}
			</Table>
		</Row>
		<Row
			caption="fill column: a long title on one line, cut with … (full text in the tooltip)"
			wide
		>
			<Table
				columns={titledColumns}
				rows={titledRows}
				rowKey={(row: (typeof titledRows)[number]) => row.id}
				label="Issues"
			>
				{#snippet cell(row: (typeof titledRows)[number], column: TitledColumn)}
					{#if column.key === 'title'}
						<Button variant="title" href="#tables" title={row.title}>{row.title}</Button>
					{:else}
						{row.events}
					{/if}
				{/snippet}
			</Table>
		</Row>
		<Row
			caption="sort (a heading links to its order, then the reverse), selected (checkboxes), rowTone, rowTitle, selectedKey"
			wide
		>
			<Table
				columns={sortedColumns}
				rows={projects}
				rowKey={(row: SampleProject) => row.id}
				label="Projects, sortable"
				sort={{ current: 'unresolved', href: () => '#tables' }}
				bind:selected={sampleSelected}
				selectLabel={(row: SampleProject) => `Select ${row.name}`}
				rowTone={sampleTone}
				rowTitle={(row: SampleProject) => `${row.unresolved} unresolved`}
				selectedKey={3}
			>
				{#snippet cell(row: SampleProject, column: ProjectColumn)}
					{#if column.key === 'project'}
						<Badge tone="project" label={row.name} />
					{:else if column.key === 'dsn'}
						<code class="break-all">{row.dsn}</code>
					{:else if column.key === 'unresolved'}
						{row.unresolved}
					{:else if column.key === 'levels'}
						<LevelSummary kind="event" chosen={row.levels} />
					{:else}
						<RowMenu label="Actions for {row.name}" items={sampleMenu} />
					{/if}
				{/snippet}
			</Table>
			<p class="mt-2 text-xs text-muted">Checked: {[...sampleSelected].join(', ') || 'none'}.</p>
		</Row>
		<Row
			caption="headerless (headings for screen readers only), rowHeader (a key–value table)"
			wide
		>
			<Table
				columns={contextColumns}
				rows={contexts}
				rowKey={(row: Context) => row.name}
				label="Contexts"
				headerless
				compact
			>
				{#snippet cell(row: Context, column: ContextColumn)}{row[column.key]}{/snippet}
			</Table>
		</Row>
		<Row caption="compact, scroll (sticky header)" wide>
			<Table
				columns={breadcrumbColumns}
				rows={breadcrumbs}
				rowKey={(row: Breadcrumb) => row.time}
				label="Breadcrumbs"
				compact
				scroll
			>
				{#snippet cell(row: Breadcrumb, column: BreadcrumbColumn)}{row[column.key]}{/snippet}
			</Table>
		</Row>
		<Row caption="flat, inside a Card: no box of its own (the MCP page's connected apps)" wide>
			<Card>
				<Table
					columns={contextColumns}
					rows={contexts}
					rowKey={(row: Context) => row.name}
					label="Contexts in a card"
					flat
				>
					{#snippet cell(row: Context, column: ContextColumn)}{row[column.key]}{/snippet}
				</Table>
			</Card>
		</Row>
		<Row caption="no rows: the empty snippet" wide>
			<Table columns={projectColumns} rows={[] as SampleProject[]} label="Projects">
				{#snippet cell()}{/snippet}
				{#snippet empty()}No projects yet.{/snippet}
			</Table>
		</Row>
	</Section>

	<Section title="Page heading and empty state" id="page-heading">
		<PageHeading title="TypeError: x is undefined" tabTitle={false}>
			{#snippet leading()}<LevelBadge level="error" />{/snippet}
			{#snippet actions()}
				<Button>Resolve</Button>
			{/snippet}
		</PageHeading>
		<form
			onsubmit={(event) => {
				event.preventDefault();
				sampleTitleError =
					sampleTitleValue === 'taken' ? 'Type anything but "taken": this one is refused.' : '';
				if (!sampleTitleError) sampleTitle = sampleTitleValue;
			}}
		>
			<PageHeading title={sampleTitle} tabTitle={false}>
				{#snippet heading()}
					<EditableTitle
						title={sampleTitle}
						bind:value={sampleTitleValue}
						label="Project name"
						error={sampleTitleError}
					/>
				{/snippet}
				{#snippet actions()}<Button variant="primary" type="submit">Save</Button>{/snippet}
			</PageHeading>
		</form>
		<PageHeading title="Projects" tabTitle={false}>
			{#snippet actions()}
				<Button variant="primary" icon="add" href="#page-heading">Add project</Button>
			{/snippet}
		</PageHeading>
		<EmptyState>No issues match the filters.</EmptyState>
		<EmptyState variant="card">
			<p>No apps connected yet.</p>
			<p class="text-xs text-muted">EmptyState variant card: a card, the text on the left.</p>
		</EmptyState>
	</Section>

	<Section title="Not found">
		<div class="rounded-card border border-dashed border-border">
			<NotFound />
		</div>
	</Section>
</div>
