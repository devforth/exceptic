import tailwindcss from '@tailwindcss/vite';
import { svelteTesting } from '@testing-library/svelte/vite';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

// `pnpm dev` (run by make dev, tools/dev.sh) serves the app and sends the Go server's own
// addresses (its API, ingest, MCP, OAuth, health, the links that stop emails) to it with the Host the browser used: the Origin
// check of /api/ui compares with it, and DSNs and OAuth are built from it. changeOrigin alone does
// not keep it (Vite 8's proxy sends the target's host), so the header is set again.
const goServer = process.env.EXCEPTIC_URL ?? 'http://localhost:8000';
const goAddresses =
	'^/(api/|mcp(/|\\?|$)|health(\\?|$)|oauth/(token|register|jwks)(\\?|$)|\\.well-known/|unsubscribe/|auth/google)';

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
		proxy: {
			[goAddresses]: {
				target: goServer,
				changeOrigin: false,
				configure: (proxy) =>
					proxy.on('proxyReq', (request, incoming) => {
						if (incoming.headers.host) request.setHeader('host', incoming.headers.host);
					})
			}
		}
	},
	// Tests of the client code and of the components in a simulated browser (pnpm test, part
	// of make web-check).
	test: {
		include: ['src/**/*.test.ts'],
		environment: 'jsdom',
		setupFiles: ['src/test/setup.ts']
	}
});
