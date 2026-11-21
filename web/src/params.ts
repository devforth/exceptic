// The matchers of route parameters ([id=projectId], [id=connectionId]).
import { defineParams } from '@sveltejs/kit/params';

/** A project ID: a positive number, so a name never reaches the lookup by name or ID. */
export function projectId(param: string): number | undefined {
	return /^[1-9]\d{0,9}$/.test(param) ? Number(param) : undefined;
}

/** A connection id, as store.NewID makes them: never "new" or "log". */
export function connectionId(param: string): string | undefined {
	return /^[a-z0-9]{8}$/.test(param) ? param : undefined;
}

export const params = defineParams({ projectId, connectionId });
