<!--
@component
Every component with its variants, on sample data. Served only by the dev server
(routes/design-system); the build test fails if the marker id below reaches dist.
-->
<script lang="ts">
	import { levels } from '#lib/api/catalog.gen.ts';
	import Badge from '#lib/components/Badge.svelte';
	import type { BadgeTone } from '#lib/components/badge.ts';
	import Button from '#lib/components/Button.svelte';
	import type { ButtonVariant } from '#lib/components/button.ts';
	import ActiveFilters from '#lib/components/ActiveFilters.svelte';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import Card from '#lib/components/Card.svelte';
	import Choice from '#lib/components/Choice.svelte';
	import ChoiceGroup from '#lib/components/ChoiceGroup.svelte';
	import CodeBlock from '#lib/components/CodeBlock.svelte';
	import type { Confirmation } from '#lib/components/confirm.ts';
	import ConfirmButton from '#lib/components/ConfirmButton.svelte';
	import CopyButton from '#lib/components/CopyButton.svelte';
	import CopyField from '#lib/components/CopyField.svelte';
	import DateRange from '#lib/components/DateRange.svelte';
	import Details from '#lib/components/Details.svelte';
	import Dialog from '#lib/components/Dialog.svelte';
	import Facts from '#lib/components/Facts.svelte';
	import FilterBar from '#lib/components/FilterBar.svelte';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import Hint from '#lib/components/Hint.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import LevelBadge from '#lib/components/LevelBadge.svelte';
	import LevelChoices from '#lib/components/LevelChoices.svelte';
	import LevelSummary from '#lib/components/LevelSummary.svelte';
	import LogBody from '#lib/components/LogBody.svelte';
	import type { MenuItem } from '#lib/components/menu.ts';
	import NavLink from '#lib/components/NavLink.svelte';
	import NotFound from '#lib/components/NotFound.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import type { NoticeTone } from '#lib/components/notice.ts';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import Pager from '#lib/components/Pager.svelte';
	import Picker from '#lib/components/Picker.svelte';
	import RowMenu from '#lib/components/RowMenu.svelte';
	import SearchField from '#lib/components/SearchField.svelte';
	import SecretReveal from '#lib/components/SecretReveal.svelte';
	import Select from '#lib/components/Select.svelte';
	import type { StatusTone } from '#lib/components/status.ts';
	import StatusText from '#lib/components/StatusText.svelte';
	import Table from '#lib/components/Table.svelte';
	import Tabs from '#lib/components/Tabs.svelte';
	import TextField from '#lib/components/TextField.svelte';
	import Time from '#lib/components/Time.svelte';
	import UsageBar from '#lib/components/UsageBar.svelte';
	import type { RowTone, TableColumn } from '#lib/components/table.ts';
	import { icons, type IconName } from '#lib/icons.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import Row from './Row.svelte';
	import Section from './Section.svelte';

	// Literal class names, so Tailwind builds them.
	const colors = [
		{ token: 'bg', swatch: 'bg-bg' },
		{ token: 'surface', swatch: 'bg-surface' },
		{ token: 'text', swatch: 'bg-text' },
		{ token: 'muted', swatch: 'bg-muted' },
		{ token: 'border', swatch: 'bg-border' },
		{ token: 'accent', swatch: 'bg-accent' },
		{ token: 'accent-text', swatch: 'bg-accent-text' },
		{ token: 'accent-hover', swatch: 'bg-accent-hover' },
		{ token: 'hover', swatch: 'bg-hover' },
		{ token: 'focus', swatch: 'bg-focus' },
		{ token: 'code-bg', swatch: 'bg-code-bg' },
		{ token: 'current-line', swatch: 'bg-current-line' },
		{ token: 'selected', swatch: 'bg-selected' },
		{ token: 'stripe', swatch: 'bg-stripe' },
		{ token: 'error', swatch: 'bg-error' },
		{ token: 'error-bg', swatch: 'bg-error-bg' },
		{ token: 'error-badge', swatch: 'bg-error-badge' },
		{ token: 'row-error', swatch: 'bg-row-error' },
		{ token: 'row-error-even', swatch: 'bg-row-error-even' },
		{ token: 'row-success', swatch: 'bg-row-success' },
		{ token: 'row-success-even', swatch: 'bg-row-success-even' },
		{ token: 'success', swatch: 'bg-success' },
		{ token: 'success-bg', swatch: 'bg-success-bg' },
		{ token: 'warning', swatch: 'bg-warning' },
		{ token: 'warning-bg', swatch: 'bg-warning-bg' },
		{ token: 'info', swatch: 'bg-info' },
		{ token: 'info-bg', swatch: 'bg-info-bg' },
		{ token: 'info-hover', swatch: 'bg-info-hover' },
		{ token: 'debug', swatch: 'bg-debug' },
		{ token: 'debug-bg', swatch: 'bg-debug-bg' },
		{ token: 'tooltip-bg', swatch: 'bg-tooltip-bg' },
		{ token: 'tooltip-text', swatch: 'bg-tooltip-text' },
		{ token: 'project', swatch: 'bg-project' }
	];

	const iconNames = Object.keys(icons) as IconName[];
	const buttonVariants: ButtonVariant[] = ['default', 'primary', 'danger', 'info', 'link', 'ghost'];
	const badgeTones: Exclude<BadgeTone, 'project'>[] = [
		'error',
		'warning',
		'info',
		'debug',
		'plain',
		'success',
		'muted'
	];
	const noticeTones: NoticeTone[] = ['info', 'success', 'warning', 'error'];
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
	type BreadcrumbColumn = TableColumn<keyof Breadcrumb>;

	const breadcrumbs: Breadcrumb[] = Array.from({ length: 30 }, (_, index) => ({
		time: `16:27:${String(index).padStart(2, '0')}`,
		category: index % 3 === 0 ? 'http' : 'console',
		message: index % 3 === 0 ? `GET /api/orders/${index} 200` : `Loaded order ${index}`
	}));

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
	let sampleRole = $state('user');
	let sampleEmail = $state(true);
	let sampleEventLevels: string[] = $state(['fatal', 'error']);
	let sampleLogLevels: string[] = $state([]);
	let sampleName = $state('backend');
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
		}
	];

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

	const sampleRoles = [
		{ name: 'admin', description: 'projects, notifications, users and deleting issues' },
		{ name: 'user', description: 'issue statuses, logs and their own coding agents' }
	];

	type SampleFact = { label: string; value: string };

	const sampleFacts: SampleFact[] = [
		{ label: 'Events', value: '1,204' },
		{ label: 'First seen', value: '3 days ago' },
		{ label: 'Release', value: 'backend@1.4.2' },
		{ label: 'Environment', value: 'production' }
	];

	const statusOptions = [
		{ value: 'unresolved', label: 'Unresolved' },
		{ value: 'resolved', label: 'Resolved' },
		{ value: 'ignored', label: 'Ignored' }
	];

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

<div id="exceptic-design-system">
	<PageHeading title="Design system">
		{#snippet actions()}
			<Button variant="primary" icon="add">Primary action</Button>
		{/snippet}
	</PageHeading>
	<p class="text-muted">
		Every component of the app with its variants. Only the dev server serves this page.
	</p>

	<Section title="Color tokens">
		<div class="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-2">
			{#each colors as color (color.token)}
				<div class="flex items-center gap-2">
					<span class={['size-8 shrink-0 rounded-control border border-border', color.swatch]}
					></span>
					<code>{color.token}</code>
				</div>
			{/each}
		</div>
	</Section>

	<Section title="Icons">
		<div class="grid grid-cols-[repeat(auto-fill,minmax(130px,1fr))] gap-2">
			{#each iconNames as name (name)}
				<div
					class="flex flex-col items-center gap-2 rounded-popup border border-border bg-surface px-2 py-3"
				>
					<Icon {name} size={24} />
					<code class="text-muted">{name}</code>
				</div>
			{/each}
		</div>
	</Section>

	<Section title="Buttons" id="buttons">
		{#each buttonVariants as variant (variant)}
			<Row caption={`variant="${variant}"`}>
				<Button {variant}>Text</Button>
				<Button {variant} icon="edit">With icon</Button>
				<Button {variant} icon="delete" title="Icon only" />
				<Button {variant} href="#buttons">Link</Button>
				<Button {variant} disabled>Disabled</Button>
			</Row>
		{/each}
		<Row caption="busy: its action runs (clicks ignored, the focus stays)">
			<Button variant="primary" busy>Saving</Button>
			<Button variant="danger" busy>Deleting</Button>
		</Row>
	</Section>

	<Section title="Copy button and toast">
		<Row caption="CopyButton with a label, icon only; Toast">
			<CopyButton text="Copied text" title="Copy the sample" label="Copy for agent" />
			<CopyButton text="http://key@localhost:8000/1" title="Copy DSN" copied="DSN copied." />
			<Button onclick={() => toast.show('A message in the toast.')}>Show a toast</Button>
		</Row>
		<Row caption="CopyField: a read-only value with its copy button" wide>
			<CopyField
				value="http://0123456789abcdef@localhost:8000/1"
				label="DSN of backend"
				copyTitle="Copy the DSN of backend"
				copied="DSN copied."
			/>
		</Row>
	</Section>

	<Section title="Code and secrets" id="code">
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
		<Row caption="CodeBlock snippet: several lines to paste, with its copy button" wide>
			<CodeBlock
				variant="snippet"
				code={sampleSnippet}
				label="SDK setup"
				copyTitle="Copy the code"
			/>
		</Row>
		<Row caption="CopyField masked: dots until Show; the button copies the value" wide>
			<CopyField
				value="exc_0123456789abcdef0123456789abcdef"
				label="Personal token"
				copyTitle="Copy the token"
				masked
			/>
		</Row>
		<Row caption="SecretReveal: shown only once, with what to do with it" wide>
			<SecretReveal
				value="Z6R4SGSGAQYPH2TOBTAQBSUH6X"
				label="Temporary password"
				copyTitle="Copy the temporary password"
				copied="Temporary password copied to clipboard."
			>
				<b>Temporary password</b>: give it to the user. It is shown only now; they choose their own
				when they log in.
			</SecretReveal>
		</Row>
	</Section>

	<Section title="Badges" id="badges">
		<Row caption="tone">
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
		<Row caption="project tone with color: a color not saved yet (the project form's preview)">
			<Badge tone="project" label="mobile" color="#0d9488" />
		</Row>
		<Row caption="StatusText: a state as a colored word">
			{#each statusTones as tone (tone)}
				<StatusText {tone} title="A {tone} state">{tone}</StatusText>
			{/each}
		</Row>
	</Section>

	<Section title="Navigation" id="navigation">
		<Row caption="Breadcrumbs: the path above a title; Esc follows the item before the last" wide>
			<Breadcrumbs
				items={[
					{ label: 'Projects', href: '#navigation' },
					{ label: 'backend', href: '#navigation-backend' },
					{ label: 'Installation', href: '#navigation-setup' }
				]}
			/>
		</Row>
		<Row caption="Tabs: the arrows, Home and End move; param puts the tab in the address" wide>
			<Tabs label="MCP client" tabs={sampleTabs} bind:selected={sampleTab}>
				{#snippet panel(id: string)}<p class="text-muted">The panel of {id}.</p>{/snippet}
			</Tabs>
		</Row>
	</Section>

	<Section title="Confirmations and row menus" id="row-menu">
		<Row caption="ConfirmButton: asks in a ConfirmDialog (Cancel focused), then runs the action">
			<ConfirmButton confirm={sampleConfirm} action={pause}>Delete</ConfirmButton>
			<ConfirmButton
				confirm={sampleConfirm}
				action={failing}
				icon="delete"
				title="Delete, and fail"
			/>
			<ConfirmButton variant="default" confirm={sampleReset} action={pause}>
				Reset password
			</ConfirmButton>
		</Row>
		<Row caption="RowMenu: the ⋯ of a table row, items as data; Delete asks first">
			<span>worker</span>
			<RowMenu label="Actions for worker" items={sampleMenu} />
		</Row>
		<Row caption="Dialog narrow with actions (a short form)">
			<Button onclick={() => (sampleNarrowDialog = true)}>Rename</Button>
			<Dialog title="Rename project" size="narrow" bind:open={sampleNarrowDialog}>
				<TextField label="Name" name="name" bind:value={sampleName} required />
				{#snippet actions()}
					<Button onclick={() => (sampleNarrowDialog = false)}>Cancel</Button>
					<Button variant="primary" onclick={() => (sampleNarrowDialog = false)}>Save</Button>
				{/snippet}
			</Dialog>
		</Row>
	</Section>

	<Section title="Form fields" id="forms">
		<div class="grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-4">
			<Card>
				<TextField
					label="Project name"
					name="name"
					bind:value={sampleName}
					hint="Lowercase letters, digits, - and _."
				/>
				<TextField label="Name" name="display_name" note="optional: the username when empty" />
				<TextField label="Email" name="email" type="email" placeholder="you@example.com" />
				<TextField
					label="Refused name"
					name="refused"
					value="Back End"
					error="Use lowercase letters, digits, - and _."
				/>
				<TextField label="Username" name="username" value="developer" readonly />
				<TextField label="Bot token" name="bot_token" type="password" saved="1234…wxyz" />
			</Card>
			<Card>
				<ChoiceGroup legend="Role" hint="What the user may do.">
					{#each sampleRoles as role (role.name)}
						<Choice
							type="radio"
							name="role"
							value={role.name}
							label={role.name}
							description={role.description}
							checked={sampleRole === role.name}
							onchange={() => (sampleRole = role.name)}
						/>
					{/each}
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
				<Row caption="Select with its option of no choice; Hint">
					<Select
						label="Status"
						all="All statuses"
						value={sampleStatus}
						options={statusOptions}
						onchange={(value: string) => (sampleStatus = value)}
					/>
					<Hint text="Delete older logs." />
				</Row>
			</Card>
			<Card>
				<LevelChoices
					kind="event"
					legend="Error levels"
					selected={sampleEventLevels}
					onchange={(chosen: string[]) => (sampleEventLevels = chosen)}
				/>
				<LevelChoices
					kind="log"
					legend="Log levels"
					selected={sampleLogLevels}
					onchange={(chosen: string[]) => (sampleLogLevels = chosen)}
				/>
				<Row caption="LevelSummary: the chosen levels, highest first, or none">
					<LevelSummary kind="event" chosen={sampleEventLevels} />
					<LevelSummary kind="log" chosen={sampleLogLevels} />
				</Row>
			</Card>
		</div>
	</Section>

	<Section title="Details and facts" id="details">
		<Details title="Breadcrumbs" count={30}>
			<p>Rows of the section.</p>
		</Details>
		<Details title="Request">
			{#snippet summary()}<code>GET http://localhost:3500/api/debug-sentry/cause</code>{/snippet}
			<p>A detail cut to one line in the heading.</p>
		</Details>
		<Details title="Levels to record" variant="form" hint="What this project stores.">
			{#snippet summary()}<LevelSummary kind="event" chosen={sampleEventLevels} />{/snippet}
			<LevelChoices
				kind="event"
				legend="Error levels"
				selected={sampleEventLevels}
				onchange={(chosen: string[]) => (sampleEventLevels = chosen)}
			/>
		</Details>
		<Details title="Projects" variant="form" invalid>
			{#snippet summary()}<span class="text-muted">none</span>{/snippet}
			<p class="text-error">Choose at least one project (invalid: it opens).</p>
		</Details>
		<Row caption="inline: a small disclosure inside other content" wide>
			<Details title="app" variant="inline">
				<CodeBlock code={sampleJson} />
			</Details>
		</Row>
		<Row caption="Facts: a label over each value" wide>
			<Facts facts={sampleFacts}>
				{#snippet value(fact: SampleFact)}{fact.value}{/snippet}
			</Facts>
		</Row>
	</Section>

	<Section title="Card and usage bar" id="card">
		<Row caption="Card with a title; UsageBar filled to a percent" wide>
			<Card title="Storage">
				<p>Database: <b>1.2 GB</b> of 2.0 GB</p>
				<UsageBar percent={60} label="Database size against its limit" />
			</Card>
		</Row>
		<Row caption="UsageBar with segments: each project's part in its color, the rest grey" wide>
			<UsageBar
				segments={[
					{ label: 'backend: 250 MB', percent: 25, color: '#2563eb' },
					{ label: 'frontend: 110 MB', percent: 11, color: '#db2777' },
					{ label: 'Other: 40 MB', percent: 4 }
				]}
				label="backend 250 MB, frontend 110 MB, other 40 MB of 1.0 GB"
			/>
		</Row>
	</Section>

	<Section title="Filters" id="filters">
		<Row caption="FilterBar with a Picker (option snippet: badges), SearchField and DateRange" wide>
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
			</FilterBar>
		</Row>
		<Row caption="ActiveFilters: a chip per field, each value removable" wide>
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
		<Row caption="Pager: the rows shown, the pages, the page size" wide>
			<Pager
				page={samplePage}
				perPage={samplePerPage}
				total={134}
				href={() => '#filters'}
				onperpage={(perPage: number) => ((samplePerPage = perPage), (samplePage = 1))}
			/>
		</Row>
	</Section>

	<Section title="Log line, time and dialog" id="log">
		<Row caption="LogBody: cut with Expand when long, Full screen with JSON fields as lines" wide>
			<LogBody text={sampleLog} />
		</Row>
		<Row caption="Time: the distance, the exact time in the tooltip">
			<Time at="2026-11-20T10:00:00Z" />
		</Row>
		<Row caption="Dialog">
			<Button onclick={() => (sampleDialog = true)}>Open a dialog</Button>
			<Dialog title="A dialog" bind:open={sampleDialog}><p>Its content scrolls inside.</p></Dialog>
		</Row>
	</Section>

	<Section title="Level badges">
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
	</Section>

	<Section title="Notices">
		{#each noticeTones as tone (tone)}
			<Notice {tone}>A {tone} notice: what happened and what to do next.</Notice>
		{/each}
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
		<PageHeading title="Projects" tabTitle={false}>
			{#snippet actions()}
				<Button variant="primary" icon="add" href="#page-heading">Add project</Button>
			{/snippet}
		</PageHeading>
		<EmptyState>No issues match the filters.</EmptyState>
	</Section>

	<Section title="Not found">
		<div class="rounded-card border border-dashed border-border">
			<NotFound />
		</div>
	</Section>
</div>
