// What the project pages share: deleting a project, the input of its form and the palette.
import { limits, projectPalette } from '#lib/api/catalog.gen.ts';
import { post, type In, type Out } from '#lib/api/client.ts';
import type { Confirmation } from '#lib/components/confirm.ts';
import type { UsageSegment } from '#lib/components/usage.ts';
import { shortDate } from '#lib/format.ts';
import { toast } from '#lib/toast.svelte.ts';

/** The question before deleting a project. */
export function deleteConfirmation(name: string): Confirmation {
	return {
		title: 'Delete project',
		message: `Delete the project ${name} with all its issues, events and logs? Its DSN stops working. This cannot be undone.`,
		confirmLabel: 'Delete'
	};
}

/** Deletes a project with its data and says so. */
export async function deleteProject(id: number): Promise<void> {
	await post('delete_project', { project: String(id) });
	toast.show('Project and its data deleted.');
}

type ProjectInput = Omit<In<'update_project'>, 'project' | 'name'>;
type LogLevel = NonNullable<ProjectInput['log_levels']>[number];

/** What the project form holds; the name has a field of its own. */
export type ProjectFormValues = {
	color: string;
	logLevels: string[];
	diskShare: number;
};

/** The server's rules of a project name (the generated limits). */
export const projectNameRules = {
	maxlength: limits.projectName,
	pattern: limits.projectNamePattern
};

/**
 * The input of create_project and update_project but the name: the whole form, levels as full
 * lists (an empty one records nothing; leaving them out would keep the stored ones).
 */
export function projectInput(values: ProjectFormValues): ProjectInput {
	return {
		color: values.color,
		// The checkboxes offer only the generated levels.
		log_levels: values.logLevels as LogLevel[],
		disk_share: values.diskShare
	};
}

/** The first palette color no project shows yet, or fallback when every one is taken. */
export function freeColor(taken: Iterable<string>, fallback: string): string {
	const used = new Set([...taken].map((color) => color.toLowerCase()));

	return projectPalette.find((entry) => !used.has(entry.color))?.color ?? fallback;
}

type Oldest = NonNullable<Out<'get_storage'>['projects']>[number]['oldest'];

/** Calendar days in the viewer's time zone from the day of `time` to the day of `now`. */
function daysBetween(time: Date, now: Date): number {
	const midnight = (date: Date) => Date.UTC(date.getFullYear(), date.getMonth(), date.getDate());

	return Math.max(0, Math.round((midnight(now) - midnight(time)) / (24 * 60 * 60 * 1000)));
}

/** How far back a project's records of each kind go: "since 10 Nov · 7 days", or none stored. */
export function keptFacts(
	oldest: Oldest | undefined,
	now = Date.now()
): { label: string; value: string; detail?: string }[] {
	const kinds: { label: string; key: keyof Oldest }[] = [
		{ label: 'Logs below error', key: 'logs_below_error' },
		{ label: 'Error logs', key: 'error_logs' },
		{ label: 'Errors', key: 'errors' }
	];

	return kinds.map(({ label, key }) => {
		const time = oldest?.[key];

		if (!time) return { label, value: 'none stored' };

		const days = daysBetween(new Date(time), new Date(now));
		const age = days === 0 ? 'today' : `${days} ${days === 1 ? 'day' : 'days'}`;

		return { label, value: age, detail: `since ${shortDate(time, true)}` };
	});
}

/**
 * The storage bar's segments: each project's part of the database in its color, then the rest,
 * in percent of the limit (of the size when there is none). None while the sizes are counted.
 */
export function usageSegments(
	storage: Out<'get_storage'>,
	colorOf: (name: string) => string | undefined
): UsageSegment[] {
	const scale = storage.limit_bytes || storage.used_bytes;

	if (storage.counting || !scale) return [];

	const percent = (bytes: number) => (bytes * 100) / scale;
	const segments: UsageSegment[] = (storage.projects ?? [])
		.filter((project) => project.bytes > 0)
		.map((project) => ({
			label: `${project.name}: ${project.size}`,
			percent: percent(project.bytes),
			color: colorOf(project.name)
		}));

	if (storage.other_bytes > 0) {
		segments.push({ label: `Other: ${storage.other}`, percent: percent(storage.other_bytes) });
	}

	return segments;
}

/** The name of a palette color, or undefined for any other color. */
export function paletteName(color: string): string | undefined {
	return projectPalette.find((entry) => entry.color === color.toLowerCase())?.name;
}
