// What a form sends of a ProjectsChoice (ProjectsChoice.svelte).

/** The projects as create_user, update_user and the connection forms take them. */
export type ProjectsInput = { all_projects: boolean; projects: string[] };

/** None with All; otherwise the chosen ids in the list's order, so a saved order counts as no change. */
export function projectsInput(
	projects: readonly { id: number }[],
	allProjects: boolean,
	projectIds: readonly string[]
): ProjectsInput {
	return {
		all_projects: allProjects,
		projects: allProjects
			? []
			: projects.map((project) => String(project.id)).filter((id) => projectIds.includes(id))
	};
}
