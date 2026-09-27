export type ToastTone = 'info' | 'success' | 'warning' | 'error';

/**
 * The one thing a toast can offer besides telling you. A cue names a trail that does not
 * exist yet, so "published" is the end of what it can say — the operator still has to
 * decide whether to go and watch it, and that is a link with a life of its own, not a
 * navigation the console performs behind their back.
 */
export interface ToastAction {
	label: string;
	onselect: () => void;
}

export interface ToastItem {
	id: number;
	tone: ToastTone;
	message: string;
	action?: ToastAction;
}

export interface ToastOptions {
	/**
	 * A toast with an action to press is a toast the operator has to find before it
	 * disappears, so it lives longer than one nobody has to act on. A caller may still
	 * pass its own value, and 0 pins it.
	 */
	timeoutMs?: number;
}

const toastToneClasses: Record<ToastTone, string> = {
	info: 'alert-info',
	success: 'alert-success',
	warning: 'alert-warning',
	error: 'alert-error'
};

export function toastClass(tone: ToastTone): string {
	return toastToneClasses[tone];
}

/** A plain toast is a receipt. A toast with something to press is a question. */
export const toastActionTimeoutMs = 12000;

export function createToastStore(timeoutMs = 4000) {
	let items = $state<ToastItem[]>([]);
	let nextId = 0;

	function dismiss(id: number): void {
		items = items.filter((item) => item.id !== id);
	}

	function push(tone: ToastTone, message: string, options: ToastOptions & { action?: ToastAction } = {}): number {
		const id = nextId++;
		items = [...items, { id, tone, message, action: options.action }];
		const life = options.timeoutMs ?? (options.action ? toastActionTimeoutMs : timeoutMs);
		if (life > 0) setTimeout(() => dismiss(id), life);
		return id;
	}

	return {
		get items(): ToastItem[] {
			return items;
		},
		dismiss,
		push
	};
}

export type ToastStore = ReturnType<typeof createToastStore>;

export const toastStore = createToastStore();
