import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// While the server-rendered UI keeps the root, the app lives under this path (web.Base in web.go).
const base = '/next';

// `pnpm dev` serves the app and sends every other request to the Go server.
const goServer = process.env.EXCEPTIC_URL ?? 'http://localhost:8000';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// dist/spa is embedded in the binary (web.go); dist/.gitkeep lets Go build without it.
			adapter: adapter({ pages: 'dist/spa', fallback: '200.html' }),
			paths: { base }
		})
	],
	server: {
		proxy: { [`^/(?!${base.slice(1)}(/|$))`]: goServer }
	}
});
