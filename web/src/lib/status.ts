export type StatusTone = 'info' | 'success' | 'warning' | 'error' | 'neutral';

export const statusToneClasses = {
	info: 'badge-info',
	success: 'badge-success',
	warning: 'badge-warning',
	error: 'badge-error',
	neutral: 'badge-neutral'
} as const satisfies Record<StatusTone, string>;

export const statusToneTextClasses = {
	info: 'text-info',
	success: 'text-success',
	warning: 'text-warning',
	error: 'text-error',
	neutral: 'text-neutral'
} as const satisfies Record<StatusTone, string>;

const statusTones: Record<string, StatusTone> = {
	running: 'info',
	interacting: 'info',
	connected: 'info',
	live: 'info',
	active: 'info',
	queued: 'info',
	success: 'success',
	approved: 'success',
	merged: 'success',
	ready: 'success',
	completed: 'success',
	announced: 'success',
	open: 'success',
	// A worktree with nothing uncommitted in it.
	clean: 'success',
	waiting_review: 'warning',
	pending: 'warning',
	reconnecting: 'warning',
	low: 'warning',
	// A checkout with uncommitted changes, and a branch nothing claims any more, both want a sweep.
	dirty: 'warning',
	leftover: 'warning',
	failed: 'error',
	rejected: 'error',
	killed: 'error',
	blocked: 'error',
	cancelled: 'error',
	disconnected: 'error',
	error: 'error',
	idle: 'neutral',
	stopped: 'neutral',
	stopping: 'neutral',
	unavailable: 'neutral',
	archived: 'neutral',
	planned: 'neutral',
	standing: 'neutral',
	staged: 'neutral',
	unknown: 'neutral',
	// A Telegram push mode, read as "will this interrupt me": sound is the only
	// mode that alerts, silent delivers without noise, and off is the absence of
	// a push. They take the in-flight and idle tones the table already uses, so
	// a settings page cannot invent a palette beside the rest of the console.
	sound: 'warning',
	silent: 'info',
	off: 'neutral'
};

export function statusTone(status: string): StatusTone {
	return statusTones[status.trim().toLowerCase()] ?? 'neutral';
}
