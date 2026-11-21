// The roles of RoleChoices.svelte and RoleBadge.svelte (a .ts module, so pages and eslint
// resolve the type).
import { choices } from '#lib/api/catalog.gen.ts';

/** A role an administrator may give: every role but the one superadmin's. */
export type Role = (typeof choices.create_user.role)[number];

const assignable: readonly string[] = choices.create_user.role;

export function isAssignable(role: string): role is Role {
	return assignable.includes(role);
}
