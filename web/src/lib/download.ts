/**
 * Saves a Blob to the operator's disk under `filename`. The anchor is attached to
 * the document for the click because a detached one is ignored by some browsers,
 * and removed immediately after: the download is owned by the browser, so nothing
 * of the page is left pointing at a revoked object URL.
 */
export function downloadBlob(blob: Blob, filename: string): void {
	const url = URL.createObjectURL(blob);
	const anchor = document.createElement('a');
	anchor.href = url;
	anchor.download = filename;
	anchor.rel = 'noopener';
	document.body.appendChild(anchor);
	anchor.click();
	anchor.remove();
	URL.revokeObjectURL(url);
}
