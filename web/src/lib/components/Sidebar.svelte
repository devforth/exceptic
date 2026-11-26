<!--
@component
The menu: the app name, the Monitoring and Settings groups (admin items for admins), the
theme (Dark mode), the account and Log out below a line. It collapses to icons (the choice is kept in this browser); under
800px wide it is a bar at the top.
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { send, type Out } from '#lib/api/client.ts';
	import { navGroups, type NavEntry } from '#lib/nav.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import Button from './Button.svelte';
	import NavLink from './NavLink.svelte';
	import ThemeSwitch from './ThemeSwitch.svelte';

	let { session }: { session: Out<'get_session'> } = $props();

	const collapsedKey = 'exceptic-sidebar';

	function readCollapsed() {
		try {
			return localStorage.getItem(collapsedKey) === 'collapsed';
		} catch {
			return false; // storage blocked: the full menu
		}
	}

	let collapsed = $state(readCollapsed());

	function toggle() {
		collapsed = !collapsed;

		try {
			localStorage.setItem(collapsedKey, collapsed ? 'collapsed' : 'expanded');
		} catch {
			// storage blocked: the choice lasts for this page
		}
	}

	const groups = $derived(
		navGroups.map((group) => ({
			...group,
			entries: group.entries
		}))
	);

	function address(entry: NavEntry) {
		return resolve(entry.route);
	}

	// The section of the page: an item of the app whose address starts the page's.
	function isActive(entry: NavEntry) {
		return page.url.pathname.startsWith(resolve(entry.route));
	}

	const unresolved = $derived(session.unresolved_issues);
	const account = resolve('/(app)/account');

	// The session ends on the server, then the login is loaded afresh.
	async function logOut(event: SubmitEvent) {
		event.preventDefault();

		try {
			await send('logout');
			location.assign(resolve('/login'));
		} catch {
			toast.show('Could not log out: try again.', 'error');
		}
	}
</script>

<aside
	class={[
		'flex shrink-0 border-border bg-surface',
		'max-sidebar:flex-row max-sidebar:flex-wrap max-sidebar:items-center max-sidebar:gap-1 max-sidebar:border-b max-sidebar:px-2.5 max-sidebar:pt-2 max-sidebar:pb-1.5',
		'sidebar:sticky sidebar:top-0 sidebar:h-screen sidebar:flex-col sidebar:gap-5 sidebar:border-r sidebar:py-[18px]',
		collapsed ? 'sidebar:w-[62px] sidebar:px-2' : 'sidebar:w-[236px] sidebar:px-3'
	]}
>
	<div
		class={[
			'flex min-w-0 items-center justify-between gap-1.5 max-sidebar:flex-1',
			collapsed && 'sidebar:justify-center'
		]}
	>
		<a
			href={resolve('/(app)/issues')}
			class={[
				'min-w-0 pr-1 pl-2.5 leading-tight font-bold wrap-anywhere text-text no-underline hover:no-underline',
				collapsed && 'sidebar:hidden'
			]}
		>
			{session.app_name}
		</a>
		<span class="max-sidebar:hidden">
			<Button
				variant="ghost"
				icon={collapsed ? 'sidebarExpand' : 'sidebarCollapse'}
				title="Collapse or expand the menu"
				expanded={!collapsed}
				onclick={toggle}
			/>
		</span>
	</div>

	<nav
		aria-label="Main"
		class="flex flex-1 flex-col gap-4 max-sidebar:order-1 max-sidebar:basis-full max-sidebar:flex-row max-sidebar:justify-around max-sidebar:gap-0"
	>
		{#each groups as group (group.id)}
			<div
				role="group"
				aria-labelledby="nav-{group.id}"
				class={[
					'flex flex-col gap-0.5 max-sidebar:contents',
					collapsed &&
						'sidebar:not-first:border-t sidebar:not-first:border-border sidebar:not-first:pt-3'
				]}
			>
				<span
					id="nav-{group.id}"
					class={[
						'px-2.5 pb-1 text-2xs font-semibold tracking-[.05em] text-muted uppercase max-sidebar:sr-only',
						collapsed && 'sidebar:sr-only'
					]}
				>
					{group.label}
				</span>
				{#each group.entries as entry (entry.id)}
					{@const counted = entry.countsUnresolved}
					<NavLink
						label={entry.label}
						icon={entry.icon}
						href={address(entry)}
						title={counted && unresolved ? `${entry.label}: ${unresolved} unresolved` : entry.title}
						active={isActive(entry)}
						count={counted ? unresolved : undefined}
						countLabel={counted ? `${unresolved} unresolved` : undefined}
						{collapsed}
					/>
				{/each}
			</div>
		{/each}
	</nav>

	<!-- The theme right above the account, as two items of one list. -->
	<div class="flex flex-col gap-0.5 max-sidebar:contents">
		<div class="max-sidebar:flex-none">
			<ThemeSwitch {collapsed} saved={session.theme} />
		</div>
		<nav aria-label="Account" class="max-sidebar:flex-none">
			<NavLink
				label={session.user.display_name}
				icon="account"
				href={account}
				title="Your account: {session.user.display_name} ({session.user.role})"
				active={page.url.pathname === account}
				{collapsed}
			/>
		</nav>
	</div>
	<!-- Set apart by a line in the full menu. -->
	<form
		onsubmit={logOut}
		class="max-sidebar:flex-none sidebar:-mt-2.5 sidebar:border-t sidebar:border-border sidebar:pt-2.5"
	>
		<NavLink label="Log out" icon="logout" danger {collapsed} />
	</form>
</aside>
