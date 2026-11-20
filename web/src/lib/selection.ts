// A choice of several values among known ones (Picker, LevelChoices).

/** The chosen values after one is checked or unchecked, in the known order. */
export function toggled<Value extends string>(
	known: readonly Value[],
	chosen: readonly Value[],
	value: Value,
	checked: boolean
): Value[] {
	const next = checked ? [...chosen, value] : chosen.filter((item) => item !== value);

	return known.filter((item) => next.includes(item));
}
