import { describe, expect, it, vi } from 'vitest';
import { createTaskStore } from './task.svelte';
import { taskBoard, taskDetail, taskListItem } from '../../tests/fixtures';
import type { TaskBoard, TaskDetail } from '$lib/api/types';

function harness(
	board: TaskBoard = taskBoard(),
	details: TaskDetail[] = [taskDetail()]
) {
	const listTasks = vi.fn(async () => board);
	const getTask = vi.fn(async (traceId: string, taskId: string) => {
		const found = details.find((task) => task.traceId === traceId && task.taskId === taskId);
		if (!found) throw new Error('task not found');
		return found;
	});
	const store = createTaskStore({ listTasks, getTask, pollIntervalMs: 0 });
	return { store, listTasks, getTask };
}

describe('createTaskStore', () => {
	it('reads the board once on mount, whatever start is called again', async () => {
		const { store, listTasks } = harness();
		expect(store.showSkeletons).toBe(true);

		await store.start();
		await store.start();

		expect(listTasks).toHaveBeenCalledTimes(1);
		expect(store.showSkeletons).toBe(false);
	});

	it('keeps the group order the server sent, because that order is the pipeline', async () => {
		const { store } = harness();

		await store.start();

		expect(store.groups.map((group) => group.status)).toEqual([
			'ready',
			'waiting_review',
			'failed'
		]);
		// The counts come from the server too, so a column heading is the colony-wide
		// number and not the number of cards that happened to render.
		expect(store.counts).toEqual({ ready: 1, waiting_review: 1, failed: 1 });
	});

	it('keeps the last board on screen through a later failure', async () => {
		const listTasks = vi
			.fn<() => Promise<TaskBoard>>()
			.mockResolvedValueOnce(taskBoard())
			.mockRejectedValueOnce(new Error('colony root unreadable'));
		const store = createTaskStore({ listTasks, getTask: async () => taskDetail(), pollIntervalMs: 0 });
		await store.start();

		await store.refresh();

		expect(store.lastError).toBe('colony root unreadable');
		expect(store.groups).toHaveLength(3);
	});

	it('finds a task by both ids, since a task id alone is not a key', async () => {
		const { store } = harness();
		await store.start();

		const found = store.find('trace-01a0bd6963faa14f', 'task-02');

		expect(found?.status).toBe('waiting_review');
		expect(store.find('trace-01a0bd6963faa14f', 'task-99')).toBeNull();
		// The wrong trail with a right task id is a different task, not the same one.
		expect(store.find('trace-other', 'task-02')).toBeNull();
	});

	it('reads a detail on select and keeps the board row beside it', async () => {
		const { store, getTask } = harness();
		await store.start();

		await store.select('trace-01a0bd6963faa14f', 'task-01');

		expect(getTask).toHaveBeenCalledWith('trace-01a0bd6963faa14f', 'task-01');
		expect(store.current?.body).toBe('Add an --format flag to `paseka export`.');
		// The row is what the eligibility flags come from, and it is still readable
		// while a slower detail read is in flight.
		expect(store.row?.canStart).toBe(true);
		expect(store.selectedKey).toBe('trace-01a0bd6963faa14f/task-01');
	});

	it('keeps a task on screen through a failed detail read', async () => {
		const { store } = harness(taskBoard(), []);
		await store.start();

		await store.select('trace-01a0bd6963faa14f', 'task-01');

		expect(store.detailError).toBe('task not found');
		expect(store.current).toBeNull();
		expect(store.row?.taskId).toBe('task-01');
	});

	it('ignores a slow detail that lands after another task was selected', async () => {
		// The race is real: a board is polled while a detail is in flight, and an
		// operator can click through two cards faster than the first read returns.
		// Without the id check the first task's body would appear under the second
		// task's heading, which reads as the wrong task's work.
		let releaseFirst = (): void => {};
		const first = new Promise<void>((resolve) => {
			releaseFirst = resolve;
		});
		const store = createTaskStore({
			listTasks: async () => taskBoard(),
			getTask: async (traceId, taskId) => {
				if (taskId === 'task-01') await first;
				return taskDetail({ taskId });
			},
			pollIntervalMs: 0
		});
		await store.start();

		const slow = store.select('trace-01a0bd6963faa14f', 'task-01');
		await store.select('trace-01a0bd6963faa14f', 'task-02');
		expect(store.current?.taskId).toBe('task-02');

		releaseFirst();
		await slow;
		expect(store.current?.taskId).toBe('task-02');
	});

	it('ignores a select with no ids rather than fetching an empty task', async () => {
		const { store, getTask } = harness();
		await store.start();

		await store.select('', '');

		expect(getTask).not.toHaveBeenCalled();
		expect(store.current).toBeNull();
	});

	it('polls, because a task status moves while an operator watches', async () => {
		vi.useFakeTimers();
		try {
			const listTasks = vi.fn(async () => taskBoard());
			const store = createTaskStore({
				listTasks,
				getTask: async () => taskDetail(),
				pollIntervalMs: 5000
			});

			store.start();
			await vi.advanceTimersByTimeAsync(10_000);
			expect(listTasks).toHaveBeenCalledTimes(3);

			store.stop();
			await vi.advanceTimersByTimeAsync(10_000);
			expect(listTasks).toHaveBeenCalledTimes(3);
		} finally {
			vi.useRealTimers();
		}
	});

	it('answers an empty board, whose counts the server sends as null', async () => {
		const { store } = harness(taskBoard({ groups: [], taskCounts: null }));
		await store.start();

		// Go marshals a nil map as null, and the store is the boundary that hands the
		// page a list rather than whatever the wire had.
		expect(store.counts).toEqual({});
	});

	it('keeps two trails\' identically named tasks apart', async () => {
		// `_review` is the default id for a rework task, so a colony with five trails
		// that have each been reviewed has five tasks with the same name. The board
		// groups across trails, so anything keyed on the task id alone collides.
		const rework = taskListItem({ taskId: '_review', title: 'Rework the last review' });
		const board = taskBoard({
			groups: [
				{
					status: 'ready',
					tasks: [rework, { ...rework, traceId: 'trace-019f8d632b01bb1f' }]
				}
			],
			taskCounts: { ready: 2 }
		});
		const { store } = harness(board);
		await store.start();

		expect(store.groups[0].tasks).toHaveLength(2);
		expect(store.find('trace-01a0bd6963faa14f', '_review')?.traceId).toBe(
			'trace-01a0bd6963faa14f'
		);
		expect(store.find('trace-019f8d632b01bb1f', '_review')?.traceId).toBe(
			'trace-019f8d632b01bb1f'
		);
	});

	it('flattens the groups for anything that wants every task at once', async () => {
		const { store } = harness();
		await store.start();

		expect(store.tasks.map((task) => task.taskId)).toEqual(['task-01', 'task-02', 'task-03']);
	});
});
