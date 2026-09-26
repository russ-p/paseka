import { getTask as requestTask, listTasks } from '$lib/api/client';
import type { TaskBoard, TaskDetail, TaskListItem } from '$lib/api/types';

interface TaskStoreOptions {
	listTasks?: () => Promise<TaskBoard>;
	getTask?: (traceId: string, taskId: string) => Promise<TaskDetail>;
	/** A task's status moves while an operator watches, so this page does poll. */
	pollIntervalMs?: number;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

function key(traceId: string, taskId: string): string {
	return `${traceId}/${taskId}`;
}

export function createTaskStore(options: TaskStoreOptions = {}) {
	const readBoard = options.listTasks ?? listTasks;
	const readTask = options.getTask ?? requestTask;
	const pollIntervalMs = options.pollIntervalMs ?? 5000;

	let board = $state<TaskBoard>({ groups: [], taskCounts: {} });
	let selectedTraceId = $state('');
	let selectedTaskId = $state('');
	let detail = $state<TaskDetail | null>(null);
	let detailLoading = $state(false);
	let detailError = $state('');
	let loading = $state(true);
	let error = $state('');
	let timer: ReturnType<typeof setInterval> | undefined;
	let started = false;

	async function refresh(): Promise<void> {
		try {
			board = await readBoard();
			error = '';
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	/**
	 * Plain functions rather than `$derived`, like the run store: a derived outside
	 * an effect can hand back a value computed before the last write, and a board
	 * that kept showing a moved task's old status would be worse than one that
	 * recomputes.
	 */
	function groups(): { status: string; tasks: TaskListItem[] }[] {
		return board.groups;
	}

	function counts(): Record<string, number> {
		return board.taskCounts;
	}

	function find(traceId: string, taskId: string): TaskListItem | null {
		for (const group of board.groups) {
			const hit = group.tasks.find(
				(task) => task.traceId === traceId && task.taskId === taskId
			);
			if (hit) return hit;
		}
		return null;
	}

	function current(): TaskDetail | null {
		if (!selectedTraceId || !selectedTaskId) return null;
		// The fetched detail is the better copy, but a board row keeps the page
		// readable while the detail is still in flight or after it failed.
		return detail && detail.traceId === selectedTraceId && detail.taskId === selectedTaskId
			? detail
			: null;
	}

	/** The board row for the selected task, for the fields the detail has not loaded yet. */
	function row(): TaskListItem | null {
		return find(selectedTraceId, selectedTaskId);
	}

	async function select(traceId: string, taskId: string): Promise<void> {
		if (!traceId || !taskId) return;
		selectedTraceId = traceId;
		selectedTaskId = taskId;
		detail = null;
		detailError = '';
		detailLoading = true;
		try {
			const loaded = await readTask(traceId, taskId);
			// Dropped if the operator has already moved on. Assigning it anyway would
			// leave the second task's heading over the first task's body, and the guard
			// in `current` would then hide the whole detail rather than just this
			// response — so the write itself is what has to be conditional.
			if (selectedTraceId === traceId && selectedTaskId === taskId) detail = loaded;
		} catch (cause) {
			if (selectedTraceId === traceId && selectedTaskId === taskId) {
				detail = null;
				detailError = errorMessage(cause);
			}
		} finally {
			if (selectedTraceId === traceId && selectedTaskId === taskId) detailLoading = false;
		}
	}

	function start(): void {
		if (started) return;
		started = true;
		void refresh();
		if (pollIntervalMs > 0) {
			timer = setInterval(() => void refresh(), pollIntervalMs);
		}
	}

	function stop(): void {
		started = false;
		if (timer !== undefined) clearInterval(timer);
		timer = undefined;
	}

	return {
		get groups(): { status: string; tasks: TaskListItem[] }[] {
			return groups();
		},
		get counts(): Record<string, number> {
			return counts();
		},
		get tasks(): TaskListItem[] {
			return board.groups.flatMap((group) => group.tasks);
		},
		get current(): TaskDetail | null {
			return current();
		},
		get row(): TaskListItem | null {
			return row();
		},
		get selectedKey(): string {
			return selectedTraceId === '' ? '' : key(selectedTraceId, selectedTaskId);
		},
		get showSkeletons(): boolean {
			return loading && board.groups.length === 0;
		},
		get lastError(): string {
			return error;
		},
		get detailLoading(): boolean {
			return detailLoading;
		},
		get detailError(): string {
			return detailError;
		},
		find,
		refresh,
		select,
		start,
		stop
	};
}

export type TaskStore = ReturnType<typeof createTaskStore>;
