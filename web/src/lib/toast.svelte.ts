// The one toast of the app: Toast.svelte shows it, anything may call show.
const shownFor = 5000;

let message = $state('');
let timer: ReturnType<typeof setTimeout> | undefined;

export const toast = {
	get message() {
		return message;
	},
	show(text: string) {
		clearTimeout(timer);
		// Cleared first, so a repeated message changes the live region and is announced again.
		message = '';
		requestAnimationFrame(() => {
			message = text;
			timer = setTimeout(() => (message = ''), shownFor);
		});
	}
};
