import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import ExportTraceModal from './ExportTraceModal.svelte';
import { downloadBlob } from '$lib/download';
import type { TraceExportDownload } from '$lib/api/client';

vi.mock('$lib/download', () => ({ downloadBlob: vi.fn() }));

const traceId = 'trace-export';
/** The response body for an export is a file, so the fetch stub serves raw bytes. */
function reportResponse(filename: string): Response {
	return new Response(new TextEncoder().encode('report'), {
		status: 200,
		headers: { 'Content-Disposition': `attachment; filename="${filename}"` }
	});
}

function stubFetch(handler?: () => Response): ReturnType<typeof vi.fn> {
	const mock = vi.fn(async (input: RequestInfo | URL) => {
		if (String(input).includes('/export')) return handler?.() ?? reportResponse('trace-export.html');
		return new Response('not found', { status: 404 });
	});
	vi.stubGlobal('fetch', mock);
	return mock;
}

afterEach(() => {
	vi.unstubAllGlobals();
	vi.clearAllMocks();
});

describe('ExportTraceModal', () => {
	it('opens on the HTML renderer with no include chosen', async () => {
		stubFetch();
		render(ExportTraceModal, { open: true, traceId, onclose: vi.fn() });

		const dialog = await screen.findByRole('dialog');
		expect(dialog).toHaveTextContent('Export trail');
		expect(screen.getByRole('radio', { name: 'HTML' })).toBeChecked();
		expect(screen.getByRole('radio', { name: 'Markdown' })).not.toBeChecked();
		expect(screen.getAllByRole('checkbox')).toHaveLength(7);
		expect(screen.getAllByRole('checkbox').every((box) => !(box as HTMLInputElement).checked)).toBe(true);
	});

	it('exports the chosen renderer and slices, saves the server name, and closes', async () => {
		const user = userEvent.setup();
		const fetchMock = stubFetch(() => reportResponse('paseka-export-demo-trace-md.md'));
		const onclose = vi.fn();
		const onexported = vi.fn<(filename: string) => void>();
		render(ExportTraceModal, { open: true, traceId, onclose, onexported });

		await user.click(screen.getByRole('radio', { name: 'Markdown' }));
		await user.click(screen.getByRole('checkbox', { name: /LLM usage/ }));

		await user.click(screen.getByRole('button', { name: 'Export' }));

		await waitFor(() => expect(onclose).toHaveBeenCalledTimes(1));
		expect(fetchMock).toHaveBeenCalledWith(
			'/api/traces/trace-export/export?format=md&include=usage'
		);
		const download = vi.mocked(downloadBlob);
		expect(download).toHaveBeenCalledTimes(1);
		const [blob, filename] = download.mock.calls[0] as [Blob, string];
		expect(await blob.text()).toBe('report');
		expect(filename).toBe('paseka-export-demo-trace-md.md');
		expect(onexported).toHaveBeenCalledWith('paseka-export-demo-trace-md.md');
	});

	it('toggles a slice back off before exporting', async () => {
		const user = userEvent.setup();
		const fetchMock = stubFetch(() => reportResponse('trace-export.html'));
		render(ExportTraceModal, { open: true, traceId, onclose: vi.fn() });

		const usage = screen.getByRole('checkbox', { name: /Colony config/ });
		await user.click(usage);
		await user.click(usage);
		await user.click(screen.getByRole('button', { name: 'Export' }));

		await waitFor(() =>
			expect(fetchMock).toHaveBeenCalledWith(
				'/api/traces/trace-export/export?format=html'
			)
		);
	});

	it('keeps the dialog open and shows the reason when the export fails', async () => {
		const user = userEvent.setup();
		stubFetch(() => new Response('colony not resolved', { status: 404 }));
		const onclose = vi.fn();
		render(ExportTraceModal, { open: true, traceId, onclose });

		await user.click(screen.getByRole('button', { name: 'Export' }));

		expect(await screen.findByRole('alert')).toHaveTextContent('colony not resolved');
		expect(onclose).not.toHaveBeenCalled();
		expect(vi.mocked(downloadBlob)).not.toHaveBeenCalled();
	});

	it('renders nothing while closed', () => {
		stubFetch();
		render(ExportTraceModal, { open: false, traceId, onclose: vi.fn() });

		expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
	});

	it('resets its choices on every open', async () => {
		const user = userEvent.setup();
		stubFetch();
		const onclose = vi.fn();
		const { rerender } = render(ExportTraceModal, { open: true, traceId, onclose });

		await user.click(screen.getByRole('radio', { name: 'Markdown' }));
		await user.click(screen.getByRole('checkbox', { name: /LLM usage/ }));

		await rerender({ open: false, traceId, onclose });
		await rerender({ open: true, traceId, onclose });

		await waitFor(() => expect(screen.getByRole('radio', { name: 'HTML' })).toBeChecked());
		expect(screen.getByRole('radio', { name: 'Markdown' })).not.toBeChecked();
		expect(
			screen.getAllByRole('checkbox').every((box) => !(box as HTMLInputElement).checked)
		).toBe(true);
	});
});