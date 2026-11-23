<!--
@component
Code or data on the code background.
- `plain`: text as given, long lines wrapped (indented JSON, a payload).
- `command`: one line to run, after an accent "›", on the page's background, with its copy button.
- `snippet`: several lines to paste (a config file), on the code background, with its copy button.
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
		class={[box, 'max-h-120 overflow-auto break-words whitespace-pre-wrap']}
		aria-label={label}>{code}</pre>
{:else if variant === 'command'}
	<div class="flex min-w-0 items-center gap-2 py-2">
		<pre
			class="min-w-0 flex-1 overflow-x-auto font-mono text-code whitespace-pre"
			aria-label={label}><span
				class="inline-block w-4 font-bold text-accent select-none"
				aria-hidden="true">›</span
			>{code}</pre>
		<CopyButton text={code} title={copyTitle} label="Copy" {copied} />
	</div>
{:else}
	<div class="flex min-w-0 items-start gap-2">
		<pre
			class={[box, 'min-w-0 flex-1 overflow-x-auto whitespace-pre']}
			aria-label={label}>{code}</pre>
		<CopyButton text={code} title={copyTitle} label="Copy" {copied} />
	</div>
{/if}
