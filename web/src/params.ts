// The matchers of route parameters ([id=projectId]).
import { defineParams } from '@sveltejs/kit/params';

/** A project ID: a positive number, so a name never reaches the lookup by name or ID. */
export function projectId(param: string): number | undefined {
	return /^[1-9]\d{0,9}$/.test(param) ? Number(param) : undefined;
}

export const params = defineParams({ projectId });
