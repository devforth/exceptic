// The keyboard in a group of items that takes one Tab stop (the tabs of a tablist, the items of
// a menu): the arrows of its direction move and wrap around, Home and End go to the ends.
export type Direction = 'horizontal' | 'vertical';

const arrows: Record<Direction, { previous: string; next: string }> = {
	horizontal: { previous: 'ArrowLeft', next: 'ArrowRight' },
	vertical: { previous: 'ArrowUp', next: 'ArrowDown' }
};

/** The index a key moves to from index (-1: none yet), or undefined for any other key. */
export function movedIndex(
	key: string,
	index: number,
	count: number,
	direction: Direction
): number | undefined {
	if (count === 0) return undefined;

	switch (key) {
		case arrows[direction].next:
			return (index + 1) % count;
		case arrows[direction].previous:
			return index <= 0 ? count - 1 : index - 1;
		case 'Home':
			return 0;
		case 'End':
			return count - 1;
		default:
			return undefined;
	}
}
