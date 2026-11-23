// Where things line up with the thumb of a Slider.

/** The width of a Slider's thumb in px. */
export const thumbSize = 20;

/**
 * The centre of a range input's thumb at value, as a CSS length from its left edge: the thumb
 * travels the width minus its own, so the start sits half a thumb in and the end half a thumb
 * short.
 */
export function thumbCenter(value: number, min: number, max: number, thumb = thumbSize): string {
	const fraction = max > min ? (Math.min(Math.max(value, min), max) - min) / (max - min) : 0;
	const offset = (0.5 - fraction) * thumb;

	return `calc(${fraction * 100}% ${offset < 0 ? '-' : '+'} ${Math.abs(offset)}px)`;
}
