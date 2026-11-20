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
