/**
 * Copy text to the clipboard, reporting whether it landed.
 *
 * `navigator.clipboard` only exists in a secure context, and the documented
 * homelab setup is plain http on a Tailscale IP or a forwarded port, so the
 * selection-based path stays as a fallback rather than an afterthought.
 */
export async function copyText(text: string): Promise<boolean> {
	if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
		try {
			await navigator.clipboard.writeText(text);
			return true;
		} catch {
			return legacyCopy(text);
		}
	}
	return legacyCopy(text);
}

function legacyCopy(text: string): boolean {
	if (typeof document === 'undefined' || !document.body) return false;
	const field = document.createElement('textarea');
	field.value = text;
	field.setAttribute('readonly', '');
	// Off-screen rather than hidden: a `display: none` field cannot be selected.
	field.style.position = 'fixed';
	field.style.opacity = '0';
	field.style.pointerEvents = 'none';
	document.body.appendChild(field);
	field.select();
	try {
		return document.execCommand('copy');
	} catch {
		return false;
	} finally {
		field.remove();
	}
}
