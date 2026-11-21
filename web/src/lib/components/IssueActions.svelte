<!--
@component
The actions on issues, each asked first: on an issue's page (its status given) Resolve and
Ignore, or Reopen; on the checked issues of the list (no status) all of them, off while none
is checked. Delete only for who may delete (the server checks it too). ondone runs after the
change, with the action.
-->
<script lang="ts">
	import { post } from '#lib/api/client.ts';
	import ConfirmButton from './ConfirmButton.svelte';
	import {
		actionLabel,
		actionsFor,
		actionVariant,
		confirmationOf,
		selectionProblem,
		type IssueAction
	} from './issueActions.ts';

	type Props = {
		issueIds: readonly string[];
		/** The status of the one issue of a page; the checked issues of a list have none. */
		status?: string;
		canDelete: boolean;
		ondone: (action: IssueAction) => unknown;
	};

	let { issueIds, status, canDelete, ondone }: Props = $props();

	const bulk = $derived(status === undefined);
	const problem = $derived(bulk ? selectionProblem(issueIds.length) : undefined);

	async function run(action: IssueAction) {
		await post('apply_issue_action', { action, issue_ids: [...issueIds] });
		await ondone(action);
	}
</script>

{#each actionsFor(status, canDelete) as action (action)}
	<ConfirmButton
		variant={actionVariant(action)}
		confirm={confirmationOf(action, bulk ? issueIds.length : undefined)}
		action={() => run(action)}
		disabled={problem !== undefined}
		title={problem}
	>
		{actionLabel(action)}
	</ConfirmButton>
{/each}
