// The page title's look and EditableTitle's types (a .ts module, so pages and eslint resolve them).

/** A page's title: the h1 of PageHeading, EditableTitle, the login and the 404 page. */
export const titleClasses = 'text-h1 font-bold wrap-break-word';

/** A title renamed in place as a field of the page's form: its name, rules and problem. */
export type TitleField = {
	/** The field's accessible name: "Project name". */
	label: string;
	/** The input name, which a refusal's field problem is matched by. */
	name?: string;
	maxlength?: number;
	pattern?: string;
	placeholder?: string;
	/** The problem of a refused value: opens the field. */
	error?: string;
};
