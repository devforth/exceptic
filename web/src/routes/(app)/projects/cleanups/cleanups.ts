import type { Out } from '#lib/api/client.ts';
import type { StatusTone } from '#lib/components/status.ts';

export type Run = NonNullable<Out<'list_cleanups'>['runs']>[number];
type Compaction = NonNullable<Run['compaction']>;

// compactionState is the Compaction cell of a run: what the files lost, or why not yet.
export function compactionState(compaction: Compaction): { text: string; tone: StatusTone } {
	switch (compaction.status) {
		case 'pending':
			return { text: compaction.error ? 'retrying…' : 'compacting…', tone: 'warning' };
		case 'done':
			return { text: `−${compaction.freed} in ${compaction.took}`, tone: 'success' };
		case 'skipped':
			return { text: 'not needed', tone: 'muted' };
		default:
			return { text: compaction.status, tone: 'error' };
	}
}

// compactionLine is the compaction in a run's details; a failed one is tried attempts times in all.
export function compactionLine(
	compaction: Compaction | null | undefined,
	stalePercent: number,
	attempts: number
): string {
	if (!compaction) return '';

	switch (compaction.status) {
		case 'pending':
			if (compaction.error)
				return `The compaction afterwards failed (${compaction.error}); it is tried again on the next pass, ${compaction.attempts ?? 1} of ${attempts} attempts made.`;
			return 'The files keep the deleted data until the compaction after the run, which comes next.';
		case 'done':
			return compaction.files_grew
				? `Compacted afterwards in ${compaction.took}: it freed ${compaction.freed}, while data arriving meanwhile took the files from ${compaction.files_before} to ${compaction.files_after}.`
				: `Compacted afterwards in ${compaction.took}: the files went from ${compaction.files_before} to ${compaction.files_after}.`;
		case 'skipped':
			return `Not compacted afterwards: what it deleted was compacted already (by an earlier compaction or the database itself), or stayed under ${stalePercent}% of the data held and is compacted with later deletes.`;
		default:
			return compaction.attempts
				? `The compaction afterwards failed ${compaction.attempts} times, the last: ${compaction.error ?? compaction.status}.`
				: `The compaction afterwards failed: ${compaction.error ?? compaction.status}.`;
	}
}

// arrivedLine is the data that came during a run, apart from what the run did; empty when it was little.
export function arrivedLine(run: Run): string {
	if (!run.arrived) return '';

	const scope = run.project ? 'the project' : 'the database';

	return `Meanwhile ${run.arrived} of new data arrived (${scope} held ${run.size_now} at the end).`;
}

// heldLine is the data a run started with and what it left; a run recorded before that was measured apart shows the files.
export function heldLine(run: Run): string {
	const compacted = run.compacted !== run.size_before ? `, ${run.compacted} after compacting` : '';

	if (run.before_files)
		return `Files: ${run.size_before} before${compacted}, so about ${run.needed} to free (recorded before the data held was measured apart from the files).`;

	return `Data held: ${run.size_before} before${compacted}, so about ${run.needed} to free; ${run.size_after} of it after (−${run.freed}).`;
}
