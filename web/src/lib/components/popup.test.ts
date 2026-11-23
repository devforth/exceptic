import { afterEach, describe, expect, it, vi } from 'vitest';
import { popupPlace } from './popup.ts';

const box = (left: number, top: number, width: number, height: number) =>
	({ left, top, width, height, right: left + width, bottom: top + height }) as DOMRect;

afterEach(() => {
	vi.restoreAllMocks();
});

describe('popupPlace', () => {
	it('puts a popup wider than its control inside the viewport', () => {
		vi.spyOn(document.documentElement, 'clientWidth', 'get').mockReturnValue(375);

		const place = popupPlace(box(140, 100, 220, 36), { width: 268, height: 300 });

		expect(place).toEqual({ top: 140, left: 375 - 268 - 8, minWidth: 220 });
	});

	it('keeps it under the control where it fits', () => {
		vi.spyOn(document.documentElement, 'clientWidth', 'get').mockReturnValue(1200);

		expect(popupPlace(box(16, 100, 220, 36), { width: 268, height: 300 }).left).toBe(16);
	});
});
