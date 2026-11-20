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
	import Card from '#lib/components/Card.svelte';
	import CopyButton from '#lib/components/CopyButton.svelte';
	import CopyField from '#lib/components/CopyField.svelte';
	import DateRange from '#lib/components/DateRange.svelte';
	import Dialog from '#lib/components/Dialog.svelte';
	import FilterBar from '#lib/components/FilterBar.svelte';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import LevelBadge from '#lib/components/LevelBadge.svelte';
	import LogBody from '#lib/components/LogBody.svelte';
	import NavLink from '#lib/components/NavLink.svelte';
	import NotFound from '#lib/components/NotFound.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import type { NoticeTone } from '#lib/components/notice.ts';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import Pager from '#lib/components/Pager.svelte';
	import Picker from '#lib/components/Picker.svelte';
	import SearchField from '#lib/components/SearchField.svelte';
	import Table from '#lib/components/Table.svelte';
	import Time from '#lib/components/Time.svelte';
	import UsageBar from '#lib/components/UsageBar.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
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
		'plain'
	];
	const noticeTones: NoticeTone[] = ['success', 'warning', 'error'];

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

	<Section title="Badges" id="badges">
		<Row caption="tone">
			{#each badgeTones as tone (tone)}
				<Badge {tone} label={tone} />
			{/each}
		</Row>
		<Row caption="project tone (its color from /project-colors.css), as a link">
			<Badge tone="project" label="backend" />
			<Badge tone="project" label="frontend" />
			<Badge tone="project" label="worker" href="#badges" title="Issues of worker" />
			<Badge tone="error" label="error" href="#badges" />
		</Row>
	</Section>

	<Section title="Card and usage bar" id="card">
		<Row caption="Card with a title; UsageBar filled to a percent" wide>
			<Card title="Storage">
				<p>Database: <b>1.2 GB</b> of 2.0 GB</p>
				<UsageBar percent={60} label="Database size against its limit" />
			</Card>
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
