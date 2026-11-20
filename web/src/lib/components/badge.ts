// The tones of Badge.svelte (a .ts module, so pages and eslint resolve the type).
export type BadgeTone =
	| 'error'
	| 'warning'
	| 'info'
	| 'debug'
	| 'plain'
	// States: on (an active connection) and off (paused).
	| 'success'
	| 'muted'
	| 'project';
