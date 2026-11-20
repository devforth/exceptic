import tailwindcss from '@tailwindcss/vite';
import { svelteTesting } from '@testing-library/svelte/vite';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { readdirSync } from 'node:fs';
import { defineConfig } from 'vitest/config';

// `pnpm dev` serves the app's pages and sends every other address to the Go server, as the
// Go server serves its own routes and leaves the rest to the app. changeOrigin stays off:
// the Origin check of /api/ui compares with the Host the browser used.
const goServer = process.env.EXCEPTIC_URL ?? 'http://localhost:8000';

/** The paths of the app's pages, read from src/routes ([param] matches one segment). */
function appPages(directory = 'src/routes', path = ''): string[] {
	return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
		if (entry.isDirectory()) {
			// A (group) folder adds nothing to the path.
			const segment = entry.name.startsWith('(') ? '' : `/${entry.name}`;

			return appPages(`${directory}/${entry.name}`, path + segment);
		}

		return entry.name.startsWith('+page.') ? [path.replace(/\[[^\]]+\]/g, '[^/]+') || '/'] : [];
	});
}

// Vite's own files (its client, modules, the sources) and the app's pages stay here.
const servedHere = ['/@', '/src/', '/node_modules/', '/.svelte-kit/', '/_app/']
	.map((prefix) => prefix.replace(/[.]/g, '\\.'))
	.concat([...new Set(appPages())].map((page) => `${page}/?(\\?|$)`));

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
			adapter: adapter({ pages: 'dist/spa', fallback: '200.html' })
		}),
		// In tests: Svelte's browser build, and the rendered components removed after each test.
		svelteTesting()
	],
	server: {
		proxy: { [`^(?!${servedHere.map((pattern) => `(${pattern})`).join('|')})`]: goServer }
	},
	// Tests of the client code and of the components in a simulated browser (pnpm test, part
	// of make web-check).
	test: {
		include: ['src/**/*.test.ts'],
		environment: 'jsdom',
		setupFiles: ['src/test/setup.ts'],
		passWithNoTests: true
	}
});
