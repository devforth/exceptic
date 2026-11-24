<!--
@component
The actions on issues, each asked first: on an issue's page (its status given) Resolve and
Ignore, or Reopen; on the checked issues of the list (no status) all of them, off while none
is checked. Ignore is a menu: for good, for a time or until a number of events more. Delete only
for who may delete (the server checks it too). ondone runs after the change, with the action.
-->
<script lang="ts">
	import { post } from '#lib/api/client.ts';
	import ConfirmButton from './ConfirmButton.svelte';
	import {
		actionLabel,
		actionsFor,
		actionVariant,
		confirmationOf,
		ignoreOptions,
		selectionProblem,
		type IgnoreBound,
		type IssueAction
	} from './issueActions.ts';
	import type { MenuItem } from './menu.ts';
	import RowMenu from './RowMenu.svelte';

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

	async function run(action: IssueAction, bound: IgnoreBound = {}) {
		await post('apply_issue_action', { action, issue_ids: [...issueIds], ...bound });
		await ondone(action);
	}

	const count = $derived(bulk ? issueIds.length : undefined);
	const ignoreItems: MenuItem[] = $derived(
		ignoreOptions.map((option, index) => ({
			label: option.label,
			icon: option.icon,
			separatorBefore: index > 0 && option.icon !== ignoreOptions[index - 1].icon,
			confirm: confirmationOf('ignore', count, option),
			action: () => run('ignore', option.bound)
		}))
	);
</script>

{#each actionsFor(status, canDelete) as action (action)}
	{#if action === 'ignore'}
		<RowMenu
			label="Ignore {bulk ? 'the selected issues' : 'this issue'}"
			text={actionLabel(action)}
			variant={actionVariant(action)}
			items={ignoreItems}
			disabled={problem !== undefined}
			title={problem}
		/>
	{:else}
		<ConfirmButton
			variant={actionVariant(action)}
			confirm={confirmationOf(action, bulk ? issueIds.length : undefined)}
			action={() => run(action)}
			disabled={problem !== undefined}
			title={problem}
		>
			{actionLabel(action)}
		</ConfirmButton>
	{/if}
{/each}
