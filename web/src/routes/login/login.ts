// What the login page says when the server sent the browser back from Google with a reason (?error=).
const googleRefusals: Record<string, string> = {
	google_failed: 'Signing in with Google did not work. Try again.',
	google_unverified:
		'Google has not verified the email of this account: sign in with your password.',
	google_no_user: 'No user has this Google account or its email. Ask an admin to add you.',
	google_not_linked:
		'Another Google account is linked to this user: sign in with that one or with your password.',
	google_unlinked:
		'Google was unlinked from this user: sign in with your password once, then Google links again.'
};

/** Why a Google sign-in was refused, from the login page's address; none without a known reason. */
export function googleRefusal(url: {
	searchParams: { get(name: string): string | null };
}): string | undefined {
	return googleRefusals[url.searchParams.get('error') ?? ''];
}
