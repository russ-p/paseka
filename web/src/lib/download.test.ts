import { afterEach, describe, expect, it, vi } from 'vitest';
import { downloadBlob } from './download';

afterEach(() => {
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

describe('downloadBlob', () => {
	it('clicks a real anchor named by the filename and revokes the object URL', () => {
		const createObjectURL = vi.fn(() => 'blob:mock/paseka');
		const revokeObjectURL = vi.fn();
		vi.stubGlobal('URL', { createObjectURL, revokeObjectURL });
		const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
		const appended = vi.spyOn(document.body, 'appendChild');
		const removed = vi.spyOn(Element.prototype, 'remove').mockImplementation(function (this: Element) {
			this.parentNode?.removeChild(this);
		});

		const blob = new Blob(['report']);
		downloadBlob(blob, 'paseka-export-demo-trail.md');

		expect(createObjectURL).toHaveBeenCalledWith(blob);
		// The anchor carries the filename and points at the object URL, is attached for
		// the click (a detached one is ignored by some browsers), and is then removed.
		const anchor = appended.mock.calls[0]?.[0] as HTMLAnchorElement;
		expect(anchor).toBeInstanceOf(HTMLAnchorElement);
		expect(anchor.download).toBe('paseka-export-demo-trail.md');
		expect(anchor.href).toBe('blob:mock/paseka');
		expect(anchor.rel).toBe('noopener');
		expect(click).toHaveBeenCalledTimes(1);
		expect(removed).toHaveBeenCalledTimes(1);
		expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock/paseka');
	});

	it('leaves nothing attached to the document', () => {
		vi.stubGlobal('URL', {
			createObjectURL: vi.fn(() => 'blob:mock/paseka'),
			revokeObjectURL: vi.fn()
		});
		const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});

		downloadBlob(new Blob(['x']), 'name.html');

		expect(document.querySelector('a')).not.toBeInTheDocument();
		expect(click).toHaveBeenCalledTimes(1);
	});
});