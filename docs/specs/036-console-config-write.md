# Spec 036: Editing Colony Configuration from Queen Console

## Status

**(Draft)**
The write half of user story #9 from [035-queen-console-redesign](035-queen-console-redesign.md): the console can edit machine-local configuration, and every refusal it reports names the source that outranks the write.

## Problem Statement

A Beekeeper whose console reports `nats.url` with the source `config.yaml` has been told where the value came from and nothing about how to change it. To move the colony to a different bus, or to turn a Telegram push category from `off` to `sound`, they have to leave the browser, edit `~/.config/paseka/<slug>/config.yaml` in a text editor, restart, and reload the page to find out whether it took.

The honest report the Settings route now gives is also a dead end, because half these settings are not decided by the file an operator would edit. `PASEKA_NATS_URL` outranks the home config, `PASEKA_PROFILE` and a `--profile` flag outrank its sticky `profile:`, and an adapter's credential is the *name* of an environment variable that the runtime resolves at call time. So an operator can edit the file, watch nothing change, and have no way to learn which of the two things is in force — which is precisely the question the page was built to answer.

The read half is not the problem, and this spec does not change it. It is that a page which names the thing deciding a value offers no way to move it, which is what makes it a report rather than a console.

## Solution

Queen Console gains an edit mode over **machine-local** configuration. A Beekeeper changes a NATS URL, a gateway push mode, or the variable name an adapter reads its API key from, saves, and gets back the new effective value **with the source that now decides it** — so the row re-renders from the server's answer rather than from what was typed, and provenance stays true after a write.

The rules that make it safe are the same rules that make it honest:

- **A save merges into the YAML.** Comments, key order, and hand-added keys survive, and the file keeps its permissions.
- **A value something outranks is refused with a 409 naming both sources**, not accepted with a 200 that changed nothing. That is the same refusal the read half already predicts, and the precedent is the git page's pull.
- **Project configuration stays read-only.** `.paseka/colony.yaml` is tracked and the CLI owns it; a browser does not write a tracked file.
- **A credential is a name, never a key.** Adapters store `api_key_env`, and the form writes a name. The gateway's `bot_token` is already a secret on disk, so it is not writable from a browser at all — it is the one setting the console reads but cannot write, and it says so.
- **A transport change says it needs a restart**, because the chrome stream's live subscription is already connected and re-subscribing it under running bees is a separate capability.
- **Everything else applies to the console you are looking at**, without a restart: the write updates the configuration the process is holding, so the next request reads it.

## User Stories

1. As a Beekeeper, I want to edit the NATS URL on the Settings page, so that I can point the colony at a different bus without leaving the browser.
2. As a Beekeeper whose NATS URL is set by `PASEKA_NATS_URL`, I want a save there to be refused with both sources named, so that I learn the file is not what decides it instead of believing my edit worked.
3. As a Beekeeper, I want to set a gateway push category from `off` to `sound`, so that a review waiting on me interrupts me.
4. As a Beekeeper, I want to point an adapter at a different environment variable for its API key, so that a new credential works without me editing YAML by hand.
5. As a Beekeeper, I want the adapter form to take a variable *name* and never a key value, so that a secret cannot travel through a browser form into a file.
6. As a Beekeeper, I want the gateway token to stay a value I set by editing the file, so that the console never becomes a way to read or write a secret it already holds on disk.
7. As a Beekeeper who hand-edits `config.yaml` with comments in it, I want those comments and my other keys to survive a save from the console, so that the file stays mine.
8. As a Beekeeper, I want the file's permissions to survive a save, so that a `0600` home config does not silently become `0644`.
9. As a Beekeeper, I want a save to change one field rather than rewrite the file from the form, so that two open consoles cannot undo each other's unrelated edits.
10. As a Beekeeper who edited the file in a text editor a minute ago, I want the console to tell me the file changed underneath it and refuse, so that a browser save cannot overwrite notes I just wrote.
11. As a Beekeeper, I want a save to be reversible from the page, so that a value I did not mean to change can be put back without opening an editor.
12. As a Beekeeper, I want the row to show the value and the source the server reports after a save, so that what the page says is what is in force rather than what I typed.
13. As a Beekeeper who changed the transport, I want the page to say the live badges stay on the old connection until the console restarts, so that I do not read a stale topbar as a failed write.
14. As a Beekeeper, I want everything except the transport to apply without a restart, so that a push-mode change is live the moment I save it.
15. As a Beekeeper, I want an invalid value refused before anything is written, so that a typo cannot become a file the next boot cannot load.
16. As a Beekeeper, I want an unset setting to be settable, so that a colony that has never declared a NATS URL can be given one.
17. As a Beekeeper, I want to remove a value I no longer want, so that the file returns to declaring nothing there.
18. As a Beekeeper, I want the subject prefix and the project configuration to be visibly read-only, so that I understand which file owns them rather than looking for a control that is missing.
19. As a Beekeeper, I want the profile to be refused while a flag or an environment variable decides it, so that I do not write a sticky value that nothing reads.
20. As a Beekeeper who changes my theme, I want that to stay browser-local, so that choosing a color never writes a machine's configuration file.
21. As a Beekeeper, I want one save in flight at a time, so that two clicks cannot produce two half-written files or two competing answers.
22. As a Beekeeper, I want the Settings page to keep not polling after a save, so that the page does not become a timer that fights me.
23. As a Beekeeper on a read-only home directory, I want a clear failure naming the file and the reason, so that I know to edit it myself.
24. As a Beekeeper, I want the console never to write the tracked project configuration, so that opening a page cannot dirty my repository.
25. As a Beekeeper editing an adapter value that is currently a loader default, I want the save to write the key explicitly, so that the next read can tell the file's value from the built-in one.
26. As a maintainer, I want one writer of colony configuration outside `paseka init`, so that the file's merge and permission semantics are not implemented twice.
27. As a Beekeeper, I want a failed write to leave the previous file exactly as it was, so that a rejected save is never a half-written file.

## Implementation Decisions

### 1. What is writable, and what is not

- **Machine-local only.** The home `config.yaml` and the per-adapter `adapters/<name>.yaml` are writable, and so is the gateway's `telegram.yaml`. The project `.paseka/colony.yaml` is not, and neither is bee YAML, cue YAML, or a profile overlay: those are tracked and the CLI owns them. The rule is stated once because it is the one a future field has to be checked against — *the console writes what is this machine's, and nothing the repository shares.*
- **Within the writable files, some fields are read-only for the same reason the read half already reports them as such.** The NATS subject prefix is committed configuration. The sticky `profile:` is a selector for which overlay the process is reading, and switching it re-resolves every layer, so it is not a field a form writes.
- **`bot_token` is not writable.** Adapters solve the secret problem by storing a variable *name*; the gateway stores the secret itself. Making it writable would mean a browser posting a secret, which is the one thing the read endpoint's refusal was designed to prevent, and the answer is already on disk for anyone who needs to rotate it.

### 2. The write layer merges a document; it does not serialize a struct

- A dedicated writer reads the target file into a `yaml.Node` document, sets or inserts exactly one key, re-encodes, writes a temp file in the same directory, fsyncs, and renames over the target. Comments, key order, and hand-added keys survive because the document is edited rather than rebuilt from a Go struct.
- `paseka init` cannot be reused for this: `writeFileIfMissing` returns without touching a file that exists and its YAML is generated from templates, so there is no round-trip-capable writer in the codebase. The console's writer is the second writer of these files and it is the only one that edits in place — the two must not grow separate ideas of what a write means.
- **Permissions are preserved, not normalized.** The target's existing mode is stat'd and reused; a file that does not exist yet is created `0600` for the home config, matching what `paseka init` writes there, because the home config is the file that can carry a NATS URL and a credential variable name. A hard-coded mode would either loosen a file that already had it tighter, or fail a home directory whose harness wrote it `0644`.
- **A rejected write leaves the old file intact**, which temp-plus-rename buys for free: there is no window in which the target is a half-written document.
- **Removal is an explicit null**, so a field can go back to being undeclared rather than to being an empty string that the loader has to be taught to read as absent.

### 3. A write is one field, and it answers with the state that resulted

- The request carries a section, a key, and a whole value — not a document, and not a patch whose merge semantics would have to be specified. The server validates, refuses if something outranks it, merges, re-reads the file, and answers with the same `{ value, source }` pair the read already sends, plus the `previous` value.
- **Answering from a re-read is the point.** If the write echoed the request, the page would render a provenance the server never confirmed — the one failure the whole settings design exists to prevent, arriving through the write path instead of around it.
- **`previous` is how a save is undone**, and it is worth carrying rather than deriving: a second write of that value is the revert, and it goes through the same refusals.

### 4. Refusals, and the one that matters most

- **A value an environment variable or a flag outranks is a 409 naming both sources.** `PASEKA_NATS_URL`, `PASEKA_TELEGRAM_BOT_TOKEN`, `PASEKA_PROFILE`, and `--profile` each beat the file for the field they own, so accepting the write would return 200 and change nothing — a success that lies is worse than the dead end this spec exists to remove. The body names the file, the variable or flag, and which one to unset. `git.Pull`'s refusal is the precedent for the shape, and the read half already reports the same condition as a `source` an operator can see.
- **An invalid value is a 400 before any file is touched**, naming the field and the rule it broke: a URL that does not parse, a push mode outside the three the gate acts on, a variable name that is not an environment variable's shape.
- **A read-only field is a 409, not a 403.** Nothing about the requester is wrong; the target is. The distinction keeps the page's two refusals — "something else decides this" and "this one is not yours to write" — from being the same shape with different wording.
- **A file that cannot be written answers with its path and the OS error**, because the operator's next move is different for a read-only mount, a missing directory, and a bad path.
- **One mutation in flight**, following the console's existing store contract: a second save while one is running is refused rather than queued, because both would merge into the same file.

### 5. A concurrent hand edit is a refusal, not a merge

- The writer re-reads the file and compares it against what the page was served; a mismatch is a 409 telling the operator to reload and try again.
- The concurrency that matters here is a human in `$EDITOR` and a browser at the same time, not two browsers. Last-writer-wins on a file a person also edits is how a carefully commented configuration loses its comments, and a comment in a hand-written file is the thing most likely to have no other copy.

### 6. Applying it to the running process, and the one honest exception

- The console's API handler holds a colony context snapshotted at boot, and the write replaces it under a mutex, so every later request in that process reads the new value. Most bus paths build their client per request from the context, so a new URL is used by the next operation that needs one.
- **What does not follow is anything already connected.** The chrome stream's subscription, and any client opened before the write, stay on the connection they have. So a transport write answers that a restart is needed, and the page says the live plaques are stale until then. Re-subscribing under running bees is a capability of its own and is out of scope.
- **Everything else is live immediately**, which is what makes the feature worth the writer: a push mode, a variable name, an allow-list entry. Those are read per use, so nothing has to be restarted for the change to be true.

### 7. Writing a defaulted value writes the key

- Every adapter loader fills in a default for a key the file omits, so a loaded struct cannot tell a declared value from a built-in one — which is why the read reports which keys a file actually sets. A save to such a field writes the key explicitly, so the next read attributes it to the file rather than to the default. Otherwise the value an operator just typed would still be reported as a default, which is the provenance bug the read half was built to eliminate.

### 8. The page: an edit mode, not a second page

- Read mode ships exactly as it does today and is not re-litigated; an **Edit** control on a writable row turns the value into an input in place, with save and cancel beside it, and a row that is read-only carries no control at all.
- **A save renders the server's answer**, including a changed source and the restart note, rather than the form's own optimism. A refusal keeps the input and shows both sources beside it, because the operator's next move is to unset a variable, not to retype a URL.
- **The store keeps not polling.** A write updates the snapshot from the response and nothing starts a timer; configuration is still a file a human edits, and a poll after a save would only re-read the value the response already carries.
- **The theme picker does not go through this endpoint.** It is the one setting the page already edits and it belongs to the browser, so saving it must not write a machine's file.

## Testing Decisions

- **A good test asserts the file after the write, not the struct.** The subject here is a document a human owns, so the external behavior is what survives: comments, key order, unrelated keys, permissions, and the file being intact after a rejection.
- **Writer tests** cover the merge contract: a comment above an untouched key survives; key order is preserved; a hand-added unknown key survives; a write into a file that lacks the key inserts it in place; an explicit null removes it and leaves the rest; an existing file's mode is reused and a new home config is `0600`; a target whose directory cannot be written leaves the original byte-identical; a file changed since it was read is refused; a second write of an unrelated field does not disturb the first.
- **Endpoint tests** cover the refusals the page depends on: a valid write answers `200` with the value and source **re-read from disk** rather than echoed, and with `previous`; an env-outranked field answers `409` naming both sources and writes nothing; an unparsable URL and an unknown push mode answer `400` with the field; a read-only field answers `409`; an adapter write sets the key and the next read attributes it to the file; an unwritable target answers with its path and leaves the file alone. The existing raw-body assertion carries over, because a secret field added later would not fail a struct-level test.
- **One concurrency test** runs reads and writes in parallel under the race detector, because the guarded context is the claim being made.
- **Component tests** cover edit mode as external behavior: no control on a read-only row, save unavailable while the value is invalid, the row rendering the response rather than the form, revert restoring `previous`, the restart note appearing for transport and not for a push mode, a 409 keeping the input and showing both sources, and a second save refused while one is in flight.
- Prior art: the console handler tests that grep a raw response body for `:null` and for Go field names, the settings store tests for poll-free reads, and the git endpoint tests for the refusal shape.

## Out of Scope

- Writing the project `.paseka/colony.yaml`, bee YAML, cue YAML, or a profile overlay — the repository stays the CLI's.
- Secret *values* in either direction: no endpoint receives a key, and the gateway's existing token is not settable from the browser.
- Reconnecting the chrome stream or any open bus client after a transport change; the write says restart required instead.
- Switching the active profile from the console, for the same reason the sticky value is read-only — a flag or an environment variable outranks it, and the switch re-resolves every layer.
- A free-form YAML editor, a generic configuration API beyond the fields the page shows, and any field the read half does not already report.
- Coordination beyond the concurrent-edit refusal: no file locks, no per-field revision history, no multi-operator merge UI.
- Per-bee or per-cue editing, which would make the console a second editor of the repository rather than of this machine.
- The bee **local** overlay `.paseka/bees/<role>.local.yaml`. It is gitignored and prompt-only, so it passes this spec's own "what is this machine's" rule, but it lives in the repository directory and nothing in the console reads it — the read half of the ask (a per-bee `{value, source}` pair) and the write half are both tracked in [backlog: Bee local overlay is invisible and unwritable in Console](../plans/backlog.md#bee-local-overlay-is-invisible-and-unwritable-in-console).

## Further Notes

- This is the write half of user story #9 in [035-queen-console-redesign](035-queen-console-redesign.md); the read half shipped with `/next/settings` and this spec does not change it.
- The read endpoint's `{ value, source }` shape is the contract the write answers with. A write that returned something else would give the page two sources of truth and undo the design the read half was built on.
- The widest part is not the file merge, it is the process snapshot: the console holds its colony context by value, so making a write visible to the running process is a guarded replacement rather than a re-read per call site — and the transport is the one value whose consumers are already connected rather than reading per use.
- Nothing in this codebase has ever written a configuration file after `paseka init` created it. That is why the writer's semantics are specified here rather than inherited, and why the refusal set is part of the feature rather than a follow-up.
