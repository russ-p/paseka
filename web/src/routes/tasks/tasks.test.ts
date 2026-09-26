import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Tasks from './+page.svelte';
import TaskDetail from './[traceId]/[taskId]/TaskDetail.svelte';
import { createTaskStore } from '$lib/stores/task.svelte';
import { createToastStore, type ToastStore } from '$lib/stores/toast.svelte';
import { beeList, taskBoard, taskDetail, taskListItem } from '../../tests/fixtures';
import type { Bee, TaskBoard, TaskDetail as TaskDetailType } from '$lib/api/types';

function harness(
	board: TaskBoard = taskBoard(),
	details: TaskDetailType[] = [taskDetail(), taskDetail({ taskId: 'task-02' })]
) {
	const listTasks = vi.fn(async () => board);
	const getTask = vi.fn(async (traceId: string, taskId: string) => {
		const found = details.find((task) => task.traceId === traceId && task.taskId === taskId);
		if (!found) throw new Error('task not found');
		return found;
	});
	const store = createTaskStore({ listTasks, getTask, pollIntervalMs: 0 });
	const toasts = createToastStore(0);
	return { store, toasts, listTasks, getTask };
}

function stubFetch(handler: (url: string, init?: RequestInit) => Response): void {
	vi.stubGlobal(
		'fetch',
		vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => handler(String(input), init))
	);
}

describe('tasks board', () => {
	it('renders one column per status, in the order the server sent', async () => {
		const { store } = harness();
		render(Tasks, { store });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());

		const labels = screen
			.getAllByRole('region')
			.map((region) => region.getAttribute('aria-label'))
			.filter((label): label is string => label !== null);
		expect(labels).toEqual(['ready tasks', 'waiting review tasks', 'failed tasks']);
	});

	it('says the waiting-review status in words, since the raw name is unreadable', async () => {
		const { store } = harness();
		render(Tasks, { store });
		await waitFor(() => expect(screen.getByLabelText('waiting review tasks')).toBeInTheDocument());

		expect(screen.getByText('waiting review')).toBeInTheDocument();
		expect(screen.queryByText('waiting_review')).not.toBeInTheDocument();
	});

	it('names each column in the topbar\'s panel style and badges the count', async () => {
		const { store } = harness();
		render(Tasks, { store });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());

		const column = screen.getByLabelText('ready tasks');
		const title = within(column).getByText('ready');
		// The console already has one pattern for naming a small panel of state, and a
		// second one here would read as two different ideas about the same thing.
		expect(title.className).toBe(
			'text-xs font-semibold tracking-wide text-base-content/60 uppercase'
		);
		// The count is the badge, and it is toned by the status so moving the status
		// word out of a badge and into the title costs the colour nothing.
		const count = within(column).getByText('1');
		expect(count.className).toContain('badge');
		expect(count.className).toContain('badge-success');
	});

	it('uses the server count, not the number of cards it happened to render', async () => {
		// The board is capped by the trace window, so a count can be larger than what
		// arrived, and the header is where an operator would notice that.
		const { store } = harness(
			taskBoard({
				groups: [{ status: 'ready', tasks: [taskListItem()] }],
				taskCounts: { ready: 42 }
			})
		);
		render(Tasks, { store });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());

		expect(within(screen.getByLabelText('ready tasks')).getByText('42')).toBeInTheDocument();
	});

	it('links every card to its own task page', async () => {
		const { store } = harness();
		render(Tasks, { store });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());

		const card = screen.getByRole('link', { name: /Wire the export format flag/ });
		expect(card).toHaveAttribute(
			'href',
			'/next/tasks/trace-01a0bd6963faa14f/task-01'
		);
	});

	it('shows what a card cannot hold in a column: bee, sector, dependencies, runs', async () => {
		const { store } = harness();
		render(Tasks, { store });
		await waitFor(() => expect(screen.getByLabelText('failed tasks')).toBeInTheDocument());

		const column = screen.getByLabelText('failed tasks');
		// The second line is who and how much; the dependency is its own badge, so it
		// is not printed on both.
		expect(within(column).getByText('builder · api · 3 runs')).toBeInTheDocument();
		expect(within(column).getByText('after task-01')).toBeInTheDocument();
	});

	it('does not repeat the id on a card whose title already is its id', async () => {
		const { store } = harness(
			taskBoard({
				groups: [
					{
						status: 'ready',
						tasks: [taskListItem({ title: 'trace-01a0bd6963faa14f', taskId: 'trace-01a0bd6963faa14f' })]
					}
				],
				taskCounts: { ready: 1 }
			})
		);
		render(Tasks, { store });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());

		const card = screen.getByRole('link');
		// Once as the title; the id line would be the same string again.
		expect(within(card).getAllByText('trace-01a0bd6963faa14f')).toHaveLength(1);
	});

	it('badges the server\'s own eligibility answer rather than deciding it', async () => {
		const { store } = harness();
		render(Tasks, { store });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());

		expect(screen.getByText('startable')).toBeInTheDocument();
		expect(screen.getByText('retryable')).toBeInTheDocument();
		// A gated task waiting on a human says so on its card, not only on its page.
		const gated = screen.getByLabelText('waiting review tasks');
		expect(within(gated).getByText('final review')).toBeInTheDocument();
	});

	it('bounds each column and scrolls it, because a colony is mostly history', async () => {
		// This colony keeps 29 completed tasks beside one ready one. An unbounded
		// column turns the board into a ribbon of history with the work at the top, and
		// the header count is what still says how much a scrolling column holds.
		// The class is on every column, so the one the fixture happens to fill is
		// enough to hold it.
		const { store } = harness();
		render(Tasks, { store });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());

		const list = within(screen.getByLabelText('ready tasks')).getByRole('list');
		expect(list.className).toContain('overflow-y-auto');
		expect(list.className).toContain('max-h-');
	});

	it('leaves out a status the colony has no tasks in, rather than an empty column', async () => {
		const { store } = harness(
			taskBoard({ groups: [taskBoard().groups[0]], taskCounts: { ready: 1 } })
		);
		render(Tasks, { store });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());

		expect(screen.queryByLabelText('failed tasks')).not.toBeInTheDocument();
	});

	it('says what an empty board means instead of showing an empty grid', async () => {
		const { store } = harness(taskBoard({ groups: [], taskCounts: {} }));
		render(Tasks, { store });
		await waitFor(() => expect(screen.getByText('No tasks')).toBeInTheDocument());

		expect(screen.getByText(/task.plan/)).toBeInTheDocument();
	});

	it('renders two trails that both call a task `_review`', async () => {
		// The board is colony-wide and `_review` is the default rework task id, so
		// duplicate task ids across trails are ordinary rather than a fixture's
		// mistake. Keyed on the task id alone, Svelte throws and renders nothing.
		const rework = taskListItem({ taskId: '_review', title: 'Rework the review' });
		const { store } = harness(
			taskBoard({
				groups: [
					{
						status: 'ready',
						tasks: [rework, { ...rework, traceId: 'trace-019f8d632b01bb1f' }]
					}
				],
				taskCounts: { ready: 2 }
			})
		);
		render(Tasks, { store });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());

		const links = within(screen.getByLabelText('ready tasks')).getAllByRole('link');
		expect(links.map((link) => link.getAttribute('href'))).toEqual([
			'/next/tasks/trace-01a0bd6963faa14f/_review',
			'/next/tasks/trace-019f8d632b01bb1f/_review'
		]);
	});

	it('reports a board that could not be read', async () => {
		const listTasks = vi.fn(async () => {
			throw new Error('colony root unreadable');
		});
		const store = createTaskStore({ listTasks, getTask: async () => taskDetail(), pollIntervalMs: 0 });
		render(Tasks, { store });

		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('colony root unreadable'));
	});
});

describe('new task drawer', () => {
	async function openDrawer(bees: Bee[] = beeList()) {
		stubFetch((url) => {
			if (url === '/api/bees') return new Response(JSON.stringify(bees));
			return new Response('{}');
		});
		const { store, toasts, listTasks } = harness();
		render(Tasks, { store, toasts });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());
		await userEvent.click(screen.getByRole('button', { name: /New task/ }));
		await waitFor(() => expect(screen.getByLabelText('New task')).toBeInTheDocument());
		return { store, toasts, listTasks };
	}

	it('offers the colony\'s bees, with a placeholder for the required one', async () => {
		await openDrawer();

		const select = screen.getByLabelText('Bee') as HTMLSelectElement;
		expect(screen.getByRole('option', { name: /Select a bee/ })).toBeInTheDocument();
		expect(within(select).getByRole('option', { name: /builder — cursor/ })).toBeInTheDocument();
		// Nothing is preselected, because dispatching a task to an arbitrary bee is not
		// a guess the console should make.
		expect(select.value).toBe('');
	});

	it('keeps Create disabled until a title or a body and a bee are both there', async () => {
		await openDrawer();

		const create = screen.getByRole('button', { name: 'Create' });
		expect(create).toBeDisabled();

		await userEvent.type(screen.getByLabelText('Title'), 'Do the thing');
		expect(create).toBeDisabled();

		await userEvent.selectOptions(screen.getByLabelText('Bee'), 'builder');
		expect(create).toBeEnabled();
	});

	it('accepts a body with no title, because the server does', async () => {
		await openDrawer();

		await userEvent.type(screen.getByLabelText('Body'), 'Fix the retry test.');
		await userEvent.selectOptions(screen.getByLabelText('Bee'), 'builder');

		expect(screen.getByRole('button', { name: 'Create' })).toBeEnabled();
	});

	it('lists the selected bee\'s own intents', async () => {
		await openDrawer();

		const intent = screen.getByLabelText('Intent') as HTMLSelectElement;
		// A bee with no selected declares nothing, so the list is empty rather than
		// every intent the colony knows.
		expect(within(intent).queryAllByRole('option')).toHaveLength(1);

		await userEvent.selectOptions(screen.getByLabelText('Bee'), 'builder');

		expect(within(intent).getByRole('option', { name: 'implementation' })).toBeInTheDocument();
		expect(within(intent).getByRole('option', { name: 'debugging' })).toBeInTheDocument();
	});

	it('says what each review policy means, because the three values are not self-evident', async () => {
		await openDrawer();

		expect(screen.getByText(/The bee finishes and the work is yours/)).toBeInTheDocument();
		await userEvent.selectOptions(screen.getByLabelText('Review'), 'final');
		expect(screen.getByText(/needs your sign-off/)).toBeInTheDocument();
	});

	it('sends only the fields the operator filled', async () => {
		let sent: Record<string, unknown> = {};
		stubFetch((url, init) => {
			if (url === '/api/bees') return new Response(JSON.stringify(beeList()));
			sent = JSON.parse(String(init?.body ?? '{}'));
			return new Response(
				JSON.stringify({ traceId: 'trace-1', taskId: 'task-01', bee: 'builder', autorun: true })
			);
		});
		const { store, toasts } = harness();
		render(Tasks, { store, toasts });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());
		await userEvent.click(screen.getByRole('button', { name: /New task/ }));
		await waitFor(() => expect(screen.getByLabelText('New task')).toBeInTheDocument());

		await userEvent.type(screen.getByLabelText('Title'), '  Do the thing  ');
		await userEvent.selectOptions(screen.getByLabelText('Bee'), 'builder');
		await userEvent.click(screen.getByLabelText('Start immediately'));
		await userEvent.click(screen.getByRole('button', { name: 'Create and start' }));

		await waitFor(() => expect(screen.queryByLabelText('New task')).not.toBeInTheDocument());
		expect(sent).toEqual({
			title: 'Do the thing',
			body: '',
			bee: 'builder',
			review: 'none',
			autorun: true
		});
		expect(toasts.items.at(-1)?.message).toBe('Task task-01 created and started');
	});

	it('splits a dependency list on commas and spaces', async () => {
		let sent: Record<string, unknown> = {};
		stubFetch((url, init) => {
			if (url === '/api/bees') return new Response(JSON.stringify(beeList()));
			sent = JSON.parse(String(init?.body ?? '{}'));
			return new Response(
				JSON.stringify({ traceId: 'trace-1', taskId: 'task-01', bee: 'builder' })
			);
		});
		const { store, toasts } = harness();
		render(Tasks, { store, toasts });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());
		await userEvent.click(screen.getByRole('button', { name: /New task/ }));
		await waitFor(() => expect(screen.getByLabelText('New task')).toBeInTheDocument());

		await userEvent.type(screen.getByLabelText('Title'), 'Do the thing');
		await userEvent.selectOptions(screen.getByLabelText('Bee'), 'builder');
		await userEvent.type(screen.getByLabelText('Depends on'), 'task-01, task-02  task-03');
		await userEvent.click(screen.getByRole('button', { name: 'Create' }));

		await waitFor(() => expect(sent.dependsOn).toEqual(['task-01', 'task-02', 'task-03']));
	});

	it('reports a create that the server refused, in its own words', async () => {
		stubFetch((url) => {
			if (url === '/api/bees') return new Response(JSON.stringify(beeList()));
			return new Response('task dependencies are not completed\n', { status: 400 });
		});
		const { store, toasts } = harness();
		render(Tasks, { store, toasts });
		await waitFor(() => expect(screen.getByLabelText('ready tasks')).toBeInTheDocument());
		await userEvent.click(screen.getByRole('button', { name: /New task/ }));
		await waitFor(() => expect(screen.getByLabelText('New task')).toBeInTheDocument());

		await userEvent.type(screen.getByLabelText('Title'), 'Do the thing');
		await userEvent.selectOptions(screen.getByLabelText('Bee'), 'builder');
		await userEvent.click(screen.getByRole('button', { name: 'Create' }));

		await waitFor(() =>
			expect(screen.getByRole('alert')).toHaveTextContent('task dependencies are not completed')
		);
		// The drawer stays open, because the operator's input is still the thing they
		// need to fix.
		expect(screen.getByLabelText('New task')).toBeInTheDocument();
	});

	it('explains an empty bee list rather than offering a form that cannot be filled', async () => {
		await openDrawer([]);

		expect(screen.getByText(/No launchable bees/)).toBeInTheDocument();
		expect(screen.queryByLabelText('Title')).not.toBeInTheDocument();
	});

	it('closes on Escape without creating anything', async () => {
		await openDrawer();

		await userEvent.keyboard('{Escape}');

		await waitFor(() => expect(screen.queryByLabelText('New task')).not.toBeInTheDocument());
	});
});

describe('task detail', () => {
	async function openTask(traceId = 'trace-01a0bd6963faa14f', taskId = 'task-01') {
		const h = harness();
		render(TaskDetail, { store: h.store, traceId, taskId, toasts: h.toasts });
		await waitFor(() => expect(h.getTask).toHaveBeenCalled());
		return h;
	}

	it('names the task with what it is for, and badges its status', async () => {
		await openTask();

		expect(
			await screen.findByRole('heading', { name: 'Wire the export format flag' })
		).toBeInTheDocument();
		expect(screen.getByText('ready')).toBeInTheDocument();
	});

	it('links back to the board and out to the trail', async () => {
		await openTask();

		expect(screen.getByRole('link', { name: '← Tasks' })).toHaveAttribute('href', '/next/tasks');
		const identity = screen.getByLabelText('Task identity');
		const trail = within(identity).getByRole('link', { name: 'trace-01a0bd6963faa14f' });
		expect(trail).toHaveAttribute('href', '/next/traces/trace-01a0bd6963faa14f');
	});

	it('folds the body, because reading a task is about what it did', async () => {
		await openTask();

		const body = await screen.findByText('Body');
		const details = body.closest('details');
		expect(details).not.toBeNull();
		expect(details).not.toHaveAttribute('open');
		// Folded, not absent: the text is in the document and the operator can open it.
		expect(screen.getByText(/Add an --format flag/)).toBeInTheDocument();
	});

	it('says where the ledger answered from, because disk and JetStream are different facts', async () => {
		await openTask();

		const identity = await screen.findByLabelText('Task identity');
		expect(within(identity).getByText('jetstream kv')).toBeInTheDocument();
	});

	it('links each linked run to its own page with its outcome', async () => {
		await openTask();

		const runs = await screen.findByLabelText('Linked runs');
		const link = within(runs).getByRole('link', { name: /builder/ });
		expect(link).toHaveAttribute(
			'href',
			'/next/runs/trace-01a0bd6963faa14f/run-02'
		);
		expect(within(runs).getByText('completed')).toBeInTheDocument();
	});

	it('says a task with no runs has not been picked up yet', async () => {
		const h = harness(taskBoard(), [taskDetail({ runs: [] })]);
		render(TaskDetail, { store: h.store, traceId: 'trace-01a0bd6963faa14f', taskId: 'task-01' });

		expect(await screen.findByText(/No runs yet/)).toBeInTheDocument();
	});

	it('offers Start only when the server says the task is eligible', async () => {
		const { store } = harness();
		render(TaskDetail, {
			store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: 'task-01',
			toasts: createToastStore(0)
		});
		await waitFor(() => expect(store.detailLoading).toBe(false));

		expect(screen.getByRole('button', { name: /Start/ })).toBeInTheDocument();
		expect(screen.queryByRole('button', { name: /Retry/ })).not.toBeInTheDocument();
	});

	it('publishes task.ready on Start and then refreshes the board', async () => {
		const calls: string[] = [];
		stubFetch((url) => {
			calls.push(url);
			return new Response(JSON.stringify({ traceId: 'trace-1', taskId: 'task-01', message: 'Published' }));
		});
		const h = harness();
		render(TaskDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: 'task-01',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.getTask).toHaveBeenCalled());

		await userEvent.click(screen.getByRole('button', { name: /Start/ }));

		await waitFor(() =>
			expect(calls).toContain('/api/traces/trace-01a0bd6963faa14f/tasks/task-01/start')
		);
		// The board row decides eligibility, so a start that changes it has to be
		// re-read rather than leaving a button that would now be refused.
		await waitFor(() => expect(h.listTasks.mock.calls.length).toBeGreaterThan(1));
		expect(h.toasts.items.at(-1)?.message).toBe('Published');
	});

	it('shows the server\'s refusal of a start as itself', async () => {
		stubFetch(() => new Response('task dependencies are not completed\n', { status: 400 }));
		const h = harness();
		render(TaskDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: 'task-01',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.getTask).toHaveBeenCalled());

		await userEvent.click(screen.getByRole('button', { name: /Start/ }));

		await waitFor(() =>
			expect(screen.getByRole('alert')).toHaveTextContent('task dependencies are not completed')
		);
	});

	it('says a task that is not there, and offers the board back', async () => {
		const h = harness(taskBoard(), []);
		render(TaskDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: 'task-99',
			toasts: h.toasts
		});

		expect(await screen.findByText('Task not found')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'All tasks' })).toHaveAttribute('href', '/next/tasks');
	});

	it('hides the review actions from a task no human is waiting on', async () => {
		await openTask();

		expect(screen.queryByRole('button', { name: 'Approve' })).not.toBeInTheDocument();
	});

	it('collapses the review form until asked, and keeps Reject to one box', async () => {
		const h = harness();
		render(TaskDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: 'task-02',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.getTask).toHaveBeenCalled());
		// The badge reads the polled board row, not the once-read detail, so a task
		// approved elsewhere stops saying it is waiting within one poll.
		expect(await screen.findByText('waiting review')).toBeInTheDocument();

		expect(screen.queryByLabelText('Approval summary')).not.toBeInTheDocument();
		await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));

		expect(screen.getByLabelText('Approval summary')).toBeInTheDocument();
		// Not a PR-delivered task, so there is nothing to title.
		expect(screen.queryByLabelText('PR title')).not.toBeInTheDocument();

		await userEvent.click(screen.getByRole('button', { name: 'Request changes…' }));

		expect(screen.getByLabelText('Feedback')).toBeInTheDocument();
		expect(screen.queryByLabelText('Approval summary')).not.toBeInTheDocument();
	});

	it('asks for the PR fields only when the task delivers as a pull request', async () => {
		const h = harness(taskBoard(), [taskDetail({ taskId: 'task-02', delivery: 'pr' })]);
		render(TaskDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: 'task-02',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.getTask).toHaveBeenCalled());
		await userEvent.click(await screen.findByRole('button', { name: 'Approve…' }));

		expect(screen.getByLabelText('PR title')).toBeInTheDocument();
		expect(screen.getByLabelText('PR body')).toBeInTheDocument();
		expect(screen.getByLabelText('Draft')).toBeInTheDocument();
		expect(screen.getByLabelText('Run hooks')).toBeInTheDocument();
	});

	it('approves with a summary and no commit message of its own', async () => {
		let sent: Record<string, unknown> = {};
		stubFetch((url, init) => {
			if (url.endsWith('/approve')) {
				sent = JSON.parse(String(init?.body ?? '{}'));
				return new Response(JSON.stringify({ traceId: 'trace-1', taskId: 'task-02' }));
			}
			return new Response('{}');
		});
		const h = harness();
		render(TaskDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: 'task-02',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.getTask).toHaveBeenCalled());
		await userEvent.click(await screen.findByRole('button', { name: 'Approve…' }));

		await userEvent.type(screen.getByLabelText('Approval summary'), 'Checked the flag path.');
		await userEvent.click(screen.getByRole('button', { name: 'Approve' }));

		await waitFor(() => expect(sent.summary).toBe('Checked the flag path.'));
		// An empty optional field is left out rather than sent blank, so the server
		// keeps its own default for it.
		expect(sent).not.toHaveProperty('mergeMessage');
	});

	it('rejects with feedback and says a rework task was created', async () => {
		stubFetch((url) => {
			if (url.endsWith('/reject')) {
				return new Response(
					JSON.stringify({ traceId: 'trace-1', taskId: 'task-02', reworkTaskId: 'task-05' })
				);
			}
			return new Response('{}');
		});
		const h = harness();
		render(TaskDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: 'task-02',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.getTask).toHaveBeenCalled());
		await userEvent.click(await screen.findByRole('button', { name: 'Request changes…' }));

		await userEvent.type(screen.getByLabelText('Feedback'), 'The flag needs a default.');
		await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));

		await waitFor(() =>
			expect(h.toasts.items.at(-1)?.message).toBe('Rework task task-05 created')
		);
	});
});
