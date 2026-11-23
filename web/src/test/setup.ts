// What jsdom lacks for the component tests: modal dialogs (open, close and its event).
if (!('showModal' in HTMLDialogElement.prototype)) {
	Object.assign(HTMLDialogElement.prototype, {
		showModal(this: HTMLDialogElement) {
			this.open = true;
		},
		close(this: HTMLDialogElement) {
			if (!this.open) return;

			this.open = false;
			this.dispatchEvent(new Event('close'));
		}
	});
}

// Svelte's transitions and animate:flip run through the Web Animations API, which jsdom lacks:
// an animation that ends at once.
if (!Element.prototype.animate) {
	Element.prototype.animate = function () {
		const animation = {
			onfinish: null as ((event: Event) => void) | null,
			cancel() {},
			finish() {},
			currentTime: 0,
			playState: 'finished'
		};

		queueMicrotask(() => animation.onfinish?.(new Event('finish')));

		return animation as unknown as Animation;
	};
	Element.prototype.getAnimations = () => [];
}
