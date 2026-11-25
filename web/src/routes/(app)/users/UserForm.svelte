<!--
@component
The form of a new user (email, name, role, projects) and of an existing one. The projects offered are the viewer's; a user's projects the viewer does not have are
not shown and stay as they are. save sends it; a refusal shows at its fields.
-->
<script lang="ts">
	import { help, limits } from '#lib/api/catalog.gen.ts';
	import { narrowFormClasses } from '#lib/components/field.ts';
	import FormActions from '#lib/components/FormActions.svelte';
	import ProjectsChoice from '#lib/components/ProjectsChoice.svelte';
	import { projectsInput, type ProjectsInput } from '#lib/components/projectsChoice.ts';
	import RoleChoices from '#lib/components/RoleChoices.svelte';
	import { isAssignable } from '#lib/components/role.ts';
	import TextField from '#lib/components/TextField.svelte';
	import { FormState } from '#lib/form.svelte.ts';
	import type { UserInput } from './users.ts';

	type Props = {
		/** The projects the viewer may give. */
		projects: { id: number; name: string }[];
		/** The viewer has every project, so may give All projects. */
		viewerAll: boolean;
		/** The roles the viewer may give (get_session's assignable_roles). */
		roles: readonly string[];
		/** The user as stored, their projects as the viewer is shown them; none for a new one. */
		user?: {
			email: string;
			name: string;
			role: string;
			allProjects: boolean;
			projectIds: number[];
			accessEditable: boolean;
			accessReason?: string;
			/** Why the viewer may not change the email, name and role (they lack some of the user's projects). */
			takeOverLocked?: string;
		};
		submitLabel: string;
		save: (input: UserInput) => Promise<unknown>;
	};

	let { projects, viewerAll, roles, user, submitLabel, save }: Props = $props();

	let email = $derived(user?.email ?? '');
	let name = $derived(user?.name ?? '');
	// A new user is a user unless chosen otherwise.
	let role = $derived(user && isAssignable(user.role) ? user.role : 'user');
	// A new user gets what the viewer has: All, or the viewer's projects.
	let allProjects = $derived(user ? user.allProjects : viewerAll);
	let projectIds: string[] = $derived(
		user ? user.projectIds.map(String) : viewerAll ? [] : projects.map((p) => String(p.id))
	);
	const editable = $derived(user?.accessEditable ?? true);
	const access = (): ProjectsInput | undefined =>
		editable ? projectsInput(projects, allProjects, projectIds) : undefined;

	// A saved user is sent only once changed.
	// svelte-ignore state_referenced_locally
	const form = new FormState(
		user ? () => ({ email: email.trim(), name: name.trim(), role, access: access() }) : undefined
	);

	function submit(event: SubmitEvent) {
		event.preventDefault();

		const chosen = access();

		void form.submit(() => save({ email, name, role, ...(chosen ? { access: chosen } : {}) }));
	}
</script>

<form onsubmit={submit} class={narrowFormClasses}>
	{#if user?.takeOverLocked}
		<TextField label="Email" name="email" value={email} readonly />
		<TextField label="Name" name="name" value={name} readonly description={user.takeOverLocked} />
	{:else}
		<!-- A saved user may still have a login from before emails: the server checks a changed one. -->
		<TextField
			label="Email"
			name="email"
			type={user ? 'text' : 'email'}
			bind:value={email}
			maxlength={limits.email}
			autocomplete="off"
			required
			error={form.fields.email}
		/>
		<TextField
			label="Name"
			name="name"
			bind:value={name}
			note="optional: the email when empty"
			maxlength={limits.displayName}
			autocomplete="off"
			error={form.fields.name}
		/>
	{/if}
	<RoleChoices bind:value={role} {roles} error={form.fields.role} locked={user?.takeOverLocked} />
	<ProjectsChoice
		{projects}
		bind:allProjects
		bind:projectIds
		legend="Projects the user sees"
		hint={help.fields.Users.Projects}
		allowAll={viewerAll}
		locked={editable ? undefined : (user?.accessReason ?? 'Your projects cannot change this.')}
		error={form.fields.projects ?? form.fields.all_projects}
	/>
	{#if !user}
		<p class="text-xs text-muted">
			A temporary password is generated; the user replaces it at the first login.
		</p>
	{/if}
	<FormActions {form}>{submitLabel}</FormActions>
</form>
