// See https://svelte.dev/docs/kit/types#app.d.ts
declare global {
	namespace App {
		/** What every page has: the root layout's session (missing only while it fails). */
		interface PageData {
			session?: import('#lib/api/client.ts').Out<'get_session'>;
		}
	}
}

export {};
