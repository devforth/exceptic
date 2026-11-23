import { render, screen } from '@testing-library/svelte';
import { fireEvent } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import Slider from './Slider.svelte';
import { thumbCenter, thumbSize } from './slider.ts';

describe('thumbCenter', () => {
	it('puts the ends half a thumb inside the track', () => {
		expect(thumbCenter(0, 0, 100)).toBe(`calc(0% + ${thumbSize / 2}px)`);
		expect(thumbCenter(100, 0, 100)).toBe(`calc(100% - ${thumbSize / 2}px)`);
	});

	it('puts the middle in the middle', () => {
		expect(thumbCenter(50, 0, 100)).toBe('calc(50% + 0px)');
	});

	it('moves the offset along with the value', () => {
		expect(thumbCenter(25, 0, 100, 20)).toBe('calc(25% + 5px)');
		expect(thumbCenter(5, 5, 100, 20)).toBe('calc(0% + 10px)');
	});

	it('keeps a value outside the scale at its end', () => {
		expect(thumbCenter(150, 0, 100, 20)).toBe('calc(100% - 10px)');
	});
});

describe('Slider', () => {
	it('puts each mark under the thumb at its value', () => {
		render(Slider, { label: 'Share', value: 40, marks: [0, 50, 100], unit: '%' });

		for (const mark of [0, 50, 100]) {
			expect(screen.getByText(`${mark}%`).style.left).toBe(thumbCenter(mark, 0, 100));
		}
	});

	it('keeps the value at its least', async () => {
		render(Slider, { label: 'Share', value: 40, least: 5 });
		const input = screen.getByLabelText<HTMLInputElement>('Share');

		await fireEvent.input(input, { target: { value: '0' } });

		expect(input.value).toBe('5');
	});
});
