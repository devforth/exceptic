// The matchers of route parameters ([id=connectionId]).
import { defineParams } from '@sveltejs/kit/params';

export const params = defineParams({
	/** A connection id, as store.NewID makes them: never "new" or "log". */
	connectionId: (param: string) => (/^[a-z0-9]{8}$/.test(param) ? param : undefined)
});
