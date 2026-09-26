export type ToastTone = 'info' | 'success' | 'warning' | 'error';

export interface ToastItem {
	id: number;
	tone: ToastTone;
	message: string;
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

export function createToastStore(timeoutMs = 4000) {
	let items = $state<ToastItem[]>([]);
	let nextId = 0;

	function dismiss(id: number): void {
		items = items.filter((item) => item.id !== id);
	}

	function push(tone: ToastTone, message: string): number {
		const id = nextId++;
		items = [...items, { id, tone, message }];
		if (timeoutMs > 0) setTimeout(() => dismiss(id), timeoutMs);
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
