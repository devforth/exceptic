<!--
@component
Code or data on the code background.
- `plain`: text as given, long lines wrapped (indented JSON, a payload).
- `command`: one line to run, after a "›", with its copy button.
- `snippet`: several lines to paste (a config file), with its copy button.
Commands and snippets keep their lines and scroll sideways, so they paste as shown.
-->
<script lang="ts">
	import CopyButton from './CopyButton.svelte';

	type Props = {
		code: string;
		variant?: 'plain' | 'command' | 'snippet';
		/** The block's name for screen readers, e.g. "Claude Code command". */
		label?: string;
		/** command and snippet: what the button does, e.g. "Copy the command". */
		copyTitle?: string;
		copied?: string;
	};

	let { code, variant = 'plain', label, copyTitle = 'Copy', copied }: Props = $props();

	const box = 'rounded-control bg-code-bg p-2.5 font-mono text-code';
</script>

{#if variant === 'plain'}
	<pre
		class={[box, 'overflow-auto break-words whitespace-pre-wrap']}
		aria-label={label}>{code}</pre>
{:else}
	<div class="flex min-w-0 items-start gap-2">
		<pre
			class={[box, 'min-w-0 flex-1 overflow-x-auto whitespace-pre']}
			aria-label={label}>{#if variant === 'command'}<span
					class="text-muted select-none"
					aria-hidden="true"
					>› </span>{/if}{code}</pre>
		<CopyButton text={code} title={copyTitle} {copied} />
	</div>
{/if}
