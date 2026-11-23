// The types of PageHeading and EditableTitle (a .ts module, so pages and eslint resolve them).

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
