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
	fork: 'info',
	pinned: 'warning',
	editable: 'neutral',
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
	// A trail that cannot admit a dispatch right now. Honey is a shared-reserve
	// condition, so it is a warning about admission, not a task error.
	paused: 'warning',
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
	// Usage-page measurement states. A finished run without usage is unmeasured,
	// which is not a failure but is not a zero either; a day still holding an
	// in-flight execution is incomplete. A day with no executions stays neutral.
	unmeasured: 'warning',
	incomplete: 'info',
	noexecutions: 'neutral',
	reported: 'success',
	// A superseded allocation, not a failure, a cancellation, or a completion.
	// It stays quiet beside `planned` rather than borrowing `failed`, `cancelled`,
	// or `completed`, because it is none of them.
	skipped: 'neutral',
	unknown: 'neutral',
	// Skill store verdicts. `ok` is the servable state; `invalid` is a broken
	// procedure and must pop; `disabled` is a recorded human decision, so it stays
	// quiet rather than reading as a failure.
	ok: 'success',
	invalid: 'error',
	disabled: 'neutral',
	// Proposal verdicts. `refused` is terminal but it is also a Beekeeper's
	// decision, so it is neutral; `missing` is a bundle the trail lost, which is
	// a fact the reviewer has to act on.
	installed: 'success',
	refused: 'neutral',
	trialled: 'info',
	missing: 'error',
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
