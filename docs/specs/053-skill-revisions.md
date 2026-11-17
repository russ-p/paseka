# Spec 053: Skill revisions

## Status

**(Draft)**
Extends [spec 041](041-skills-layer.md); it does not replace it. 041 settled the store, the per-role gate, the proposal path and the console surface for a *new* skill. This spec settles the case 041 left open: a proposal whose name is already in the store, and the record of why each version of a skill exists. Read 041 first; every decision here assumes its vocabulary and its "a human approves, the console never commits" rule.

## Problem Statement

A Beekeeper reviewing proposals sees a flat list: every row says `pending`, and nothing on it says whether the proposal would add a skill or change one that is already installed. The store and the proposals are on the same page and the row still does not join them, so the reader has to remember which names are in the store and compare by eye.

Opening a revision makes that worse rather than better. The modal shows the proposed `SKILL.md` **body** and nothing else: not the installed body to compare against, not the supporting files, not the description, not what the install would write. The one question a revision asks — *what changes if I say yes?* — is answered by holding two documents in your head. A proposal that touches four of nine files reads exactly like one that rewrites everything, because the four unchanged files are invisible and the six lines of install provenance are not.

And the decision cannot be taken. The console installs a proposal through the same validated path as the shell with no force flag, so a proposal whose name is already in the store fails with *"the store already holds a skill named …; pass --force to replace it"*. The operator is told to leave the surface they are reading, in a syntax the surface never taught them, after being shown nothing that would justify the replacement.

The record is just as thin. The skill's metadata says how it arrived — source, trail, role, approved time, installed procedure hash — which describes the **first** install and then gets overwritten by the next one. Nothing in the skill says *why* the second version exists. That argument does exist while the proposal is open, in the discovery's `reason` and in the trail; but the trail is machine-local and disappears with `paseka prune`, so the motivation for a change to a committed procedure outlives its evidence by a few weeks and then is gone. A reader of the store sees a procedure with no history of argument, and `git log` can say *what* changed in a commit but never *why the change was proposed* — that sentence was never written to a committed file.

## Solution

A proposal that targets an installed skill is a **revision**, and the console says so before it is opened: the row names the installed skill it would change and whether the store has moved since the proposal was written. Opening it shows the change rather than the document:

- **the procedure diff** — exactly the bytes the procedure's identity covers (the `SKILL.md` body and every supporting file), as one patch, with files that are unchanged counted instead of listed, and binary files named without a body;
- **the declared metadata** — `name`, `description` and any other key the author wrote, compared key by key, because a description change is what a role reads in the registry and a body-only diff would hide it;
- **what the install writes** — the provenance keys and the history line it would append, which is where the motivation for this change appears at the moment of decision.

The install appends that motivation to the skill itself: `metadata.history` carries one line for each version that stood over another — when, which procedure hash, from where, and why in one sentence. The first version of a name writes no line, because a single entry would only restate the provenance keys beside it; history begins at the change it exists to explain. It is frontmatter, so it is excluded from the procedure hash and from the procedure diff by construction: writing history never changes the identity of the procedure it describes, and history churn never appears inside the change a reviewer is reading.

Replacement stays a shell action in this spec. The console can now show that a replacement is what a decision means and how the store has moved; it still does not force one. What changes is that the operator is no longer asked to decide blind.

## User Stories

1. As a Beekeeper, I want a proposal row to say whether it would add a skill or change an installed one, so that I know which kind of decision I am looking at before I open anything.
2. As a Beekeeper, I want a revision row to name the installed skill it would change, so that the link between the two is the system's claim and not my memory.
3. As a Beekeeper, I want to see whether the store still holds the revision the proposal was written against, so that I do not approve a change reviewed against a version that no longer exists.
4. As a Beekeeper, I want the console to tell me the store has moved since the proposal was written, so that I know a re-read is needed rather than discovering it after a failed install.
5. As a Beekeeper, I want the same row to read `pending` whether the target is new or installed, so that the decision vocabulary stays one set of words instead of gaining a fifth state.
6. As a Beekeeper, I want a proposal for a name already installed to be indistinguishable from a no-op when the procedure hash matches, so that I can refuse it knowing nothing would change.
7. As a Beekeeper, I want to open a revision and see the procedure diff, so that I can answer "what changes if I say yes?" without comparing two documents by eye.
8. As a Beekeeper, I want the diff to cover the same bytes the procedure hash covers, so that "no diff" and "same revision" are the same statement and cannot disagree.
9. As a Beekeeper, I want a file that did not change to be counted rather than rendered, so that a four-file revision of a nine-file skill does not look like a rewrite.
10. As a Beekeeper, I want added and removed files to be named as such, so that I can see scope change and not only content change.
11. As a Beekeeper, I want a binary or oversized file named with the reason it has no body, so that absence is a fact rather than a rendering failure.
12. As a Beekeeper, I want to read the diff in the console's existing unified and split layouts, so that reviewing a skill change works the way reviewing code does.
13. As a Beekeeper, I want a large patch to say it was truncated and what was left out, so that I know when to read the files instead of the diff.
14. As a Beekeeper, I want a changed `description` to be shown even though it is not part of the procedure, so that I approve the text a role will actually read.
15. As a Beekeeper, I want install-written metadata shown separately from author-written metadata, so that provenance churn is never mistaken for a change to the procedure.
16. As a Beekeeper, I want to see the exact history line an install would append, so that the motivation I am approving is the motivation that lands in the store.
17. As a Beekeeper, I want the reason a revision was proposed to be written into the skill by the install, so that the argument survives the trail's pruning.
18. As a Beekeeper, I want every version after the first to carry one line of why it exists, so that a reader of the store can follow its argument without the machine that produced it.
19. As a Beekeeper, I want history to be readable in the skill file itself, so that it travels with a bundle, an export, or a copy into another colony.
20. As a Beekeeper, I want the newest entry last, so that reading the file top to bottom reads the procedure's life in order.
21. As a Beekeeper, I want each history line to name the procedure hash and the trail it came from, so that I can trace a version back to the run that argued for it.
22. As a Beekeeper, I want an over-long reason to be shortened in the line rather than rejected by the install, so that one verbose proposal cannot block an approval.
23. As a Beekeeper, I want a hand-written skill to have no history and to say so, so that "no record" is not mistaken for "no changes".
24. As a Beekeeper, I want a hand edit that nobody approved to remain `doctor`'s drift finding rather than a silent history entry, so that the record only contains decisions.
25. As a Beekeeper, I want the console to refuse a replacement it cannot do in the browser, and to name the shell command that can, so that the failure teaches the next step instead of quoting a rule.
26. As a Beekeeper, I want the store's disabled state carried into the revision view, so that I know a replacement keeps the skill off.
27. As an Archivist Bee, I want to record the procedure hash I read before proposing a revision, so that the system can tell whether the store moved under my proposal.
28. As an Archivist Bee, I want a new skill to require no base revision, so that the common case stays one payload shape.
29. As an Archivist Bee, I want a malformed base hash refused where the payload is validated and a stale one reported beside my proposal, so that a typo is caught early without a producer having to read a store that may have moved since.
30. As a Bee, I want the skill I load to describe its own history, so that I can see the procedure has been corrected before and why.
31. As an operator of the shell, I want `skills proposals` to name the same relation the console does, so that the two surfaces cannot disagree about what I am deciding.
32. As an operator of the shell, I want `skills show` to print the history, so that I can read a skill's reasoning without a git history and a trail.
33. As an operator of the shell, I want `--json` to carry the relation and the history as fields, so that a script never parses a rendered line.
34. As a maintainer, I want a revision whose bundle is missing or invalid to keep its row and say what is wrong, so that an unreadable proposal is a state and not a vanished one.
35. As a maintainer, I want the diff read to be a separate bounded request from the row read, so that listing twenty proposals does not transfer twenty bundles.
36. As a maintainer, I want no new persisted revision ledger, so that decisions stay with the trail that made them and git stays the only content history.
37. As a maintainer, I want the store's recorded hash compared with a recomputed one and reported as drift, so that the revision view never claims the installed copy is what was approved when it is not.
38. As a maintainer, I want the diff to be rendered by the console's one patch renderer, so that a second, subtly different diff implementation cannot appear on a second surface.
39. As a Beekeeper, I want a revision whose bundle happens to carry a copy of the installed history to append exactly one line, so that copying the skill before editing it cannot double its record.
40. As a Beekeeper, I want a skill's first version to write no history line, so that the block always marks a version that stood over another and the first line is never noise.
41. As a Beekeeper, I want a procedure copied from another colony to keep the history it arrived with, so that moving a skill between colonies does not silently restart its record.

## Implementation Decisions

**1. A revision is the existing procedure hash, not a new entity.** The procedure hash — the `SKILL.md` body followed by every supporting regular file in byte-sorted path order — already identifies content, and decisions are already keyed by it. No counter, no revision number, no per-skill version list, and no new store ledger are introduced. A store ledger of revisions would duplicate git, which already records what each committed version contained; the missing fact is not *what* changed but *why*, and that is decision 10.

**2. The discovery payload gains one optional field, `basedOn`: the procedure hash the proposer built on.** `INSIGHT/skill.proposed` keeps its five required fields and gains an optional sixth. Emit checks its **shape** only — a lowercase procedure hash when present, nothing when the skill is new — because the emit path has no business reading the store: a producer's store view is the worktree it runs in, and whether the base still exists is a question about the store as it is *now*, asked when someone reads the proposal. Whether the base matches anything is therefore decided by the proposal scan, not by the emit: a base the store does not hold reads `moved`, and an absent base for an installed name reads `unbased`, never an error — the proposals that already exist predate the field. The Archivist's skill intent must read the installed skill before revising it and pass the hash it read; the per-intent contract says so, and a revision that omits it is a contract finding rather than an emit failure.

**3. One relation, computed from store facts, is carried on the proposal row.** The values are `new`, `same`, `in-step`, `moved`, and `unbased`, meaning respectively: no store entry with that name; an entry whose procedure hash equals the proposal's own; an entry that matches the proposal's `basedOn`; an entry that does not; an installed name whose proposal carries no base at all. They are evaluated in that order, so a no-op reads `same` and never `in-step`, because "nothing would change" is the fact that decides the row. The relation is a *fact beside the decision*, never a new decision state — the state vocabulary stays the store's verdict plus the decision's outcome, and the page prints the relation the way it prints the destination of a trial. A relation is computed from the store as it is now, on every read, and is never recorded.

**4. The revision view needs four store facts, read once with the rest of the inventory.** For the matching name: the recorded install-time procedure hash, the recomputed hash, whether those differ (*drifted*), and whether the ledger holds a disable record. A drifted entry is reported as what it is and never blocks the diff: `doctor` owns the verdict, and a reviewer needs to see the gap, not a refusal. The install's replacement semantics are unchanged — an existing entry is still refused unless the shell forces it.

**5. The procedure diff compares exactly the bytes the procedure hash covers.** Body plus supporting files, sorted by path, with the manifest's frontmatter excluded and every other file compared whole. This is a rule with a testable consequence: two bundles have an empty procedure diff if and only if their procedure hashes are equal. Consequently the diff cannot show install provenance, cannot show history, and cannot disagree with the identity the decision is keyed on.

**6. The diff is one unified patch produced by the server, paths relative to the skill directory.** The console's existing patch renderer is the only renderer; no second diff implementation appears in the browser and no diff library is added to the server. The patch uses the conventional `a/` and `b/` sides, so the renderer's split layout, per-file navigation and line anchoring work unchanged. Producing it must not depend on a git worktree, a branch, or a commit: a skill can be diffed in a colony whose working tree is dirty and in a store that has never been committed.

**7. The file set is part of the answer.** Every path in either side is classified as changed, added, removed, or unchanged, and the unchanged ones are counted rather than rendered. A file that is binary, or past the per-file budget, is listed with the reason it has no body instead of a patch. The whole read is capped, and a capped read says so rather than passing a prefix off as the change.

**8. Declared metadata is diffed separately from the procedure.** Keys the author writes — `name`, `description`, and any other key present on either side and not install-written — are compared key by key and rendered as fields, not as a YAML text diff: a changed description is a sentence a role reads, and a frontmatter block diff would bury it among quoting differences. A key that only one side has is shown as added or removed.

**9. Install-written metadata is a third block, phrased as what the install will write.** The provenance keys (`source`, `trail`, `role`, `approved_at`, `procedure_hash`) and, when the install appends one, the new history line are shown as prospective values rather than as a diff, because the bundle never carries them and the install always writes them. This is the block that stops provenance churn from being mistaken for procedure change.

**10. The motivation of a version lives in the skill, in `metadata.history`.** One line per version after the first in the record, chronological, newest last, appended by the install:

```yaml
metadata:
  history: |
    2026-11-15T09:02:44Z 617c94be trail/trace-01a1da38673703e4 — publishes is advisory, the delivery table was wrong, the deferred proof was missing
    2026-12-02T11:20:08Z 3f91ac07 trail/trace-01a1e4c9b2f07d31 — the loader paragraph named a path the advertisement no longer prints
```

The block starts empty and the **first version of a name writes no line**: one entry restating `source`, `trail` and `approved_at` beside it is not a history, and the block earns its place only when it explains a change. A line is appended for the second and every later version, and the rule reads: a replacement always appends one, and a first install appends one only when the bundle it installs already carries history, so a procedure copied from another colony continues its record instead of restarting it. Every line therefore marks a version installed over another, and a skill whose history is empty is a skill with one recorded version.

A line is `<RFC 3339 time> <procedure hash, first 8> <origin> — <motivation>`, where the origin is `trail/<traceId>` for an approval, `nuc` for a Nuc installation, and `shell` for an install forced by hand without a proposal. The motivation is the discovery's reason, normalised to a single line and capped (240 characters, ellipsis-marked), so one verbose proposal cannot make the file unreadable or the install fail; the cap is a first number chosen to be moved by practice rather than an argued optimum, so it lives in one place and changing it is not a decision that reopens this spec. The block is a single string inside a string map, which keeps it inside the Agent Skills format and inside the store's metadata contract; a nested list is rejected (decision 17). A store entry whose history outgrows its budget is a `doctor` finding and never an install failure, on the same reasoning as drift in decision 4: a record that has become long is something to look at, not a reason to refuse a decision.

**11. History is written by the same provenance edit that already preserves everything else.** The install writes into a copy of the manifest it is installing: every frontmatter key it does not own, and the body, stay byte-for-byte. For a replacement the existing installed manifest is read first, so its history is the base and the new line is appended rather than replacing it — an install that overwrote history would delete the only record of why earlier versions exist. The bundle's own history is **not** authority for a replacement: it is the proposer's snapshot of what was installed when they read it, so its lines are ignored and the store's are kept, and a bundle that carries history indistinguishable from the store's cannot duplicate an entry. For a genuinely new skill the bundle's manifest is the base, which is how a record travels when a procedure is copied into another colony; such a copy keeps the lines it arrived with and gains one for the version this colony installs (decision 10). Nothing else about history is machine-generated.

**12. History is never part of the procedure hash, and that is why it is safe.** The procedure hash covers the body and supporting files; frontmatter is excluded. Writing history therefore cannot change the identity of the procedure it describes, cannot invalidate the hash recorded in the same frontmatter, and cannot appear as a change in the procedure diff (decision 5). A separate history *file* was rejected for the opposite reason: a supporting file is hashed, so appending to it would change the revision identity of every skill that keeps one (decision 17).

**13. Reading a revision is two reads: the facts, then the patch.** The row and detail read carries the relation, the four store facts, and the metadata blocks; the patch is a separate bounded read issued when the operator asks for it, so a list of twenty proposals transfers no bundles and a large patch cannot fail a row. A missing bundle, an invalid bundle, or a drifted store entry is reported in place: the facts block still renders and the patch block says why it cannot.

**14. The console does not gain a replacement path in this spec.** The approval plan and the approval call stay force-free, and a revision's decision view says what a replacement means and that it is a shell action. What the spec buys is that the operator can read the change and see that the store moved before leaving the console; taking the decision in the browser is a separate decision about a browser writing a tracked directory, and it is out of scope.

**15. The relation is on the row, not in a second table.** The proposals pane gains the relation as a fact on the skill cell (the installed name it would change) and the state cell keeps its vocabulary. No new page, no new column that only revisions fill: a proposal for a new skill keeps the cells it has today and reads as one.

**16. The shell reads the same projection.** `skills proposals` names the relation in its rows and carries it in `--json`; `skills show` prints the history as a labelled block and carries it in `--json`. Both are read-only additions: no verb is added, no output field is removed, and a script that parses the current shape keeps working.

**17. Rejected alternatives, recorded so they are not re-litigated.** A nested YAML list under `metadata` (`metadata.changes:` as a sequence of maps) is rejected because the store's metadata contract is a string map and the Agent Skills format defines it that way. A separate `CHANGELOG.md` inside the skill is rejected because supporting files are hashed, so history would be inside the revision identity it describes and every entry would change the hash recorded beside it. A numbered revision counter or a per-skill revision list in the store is rejected as a second history beside git. Recording history in the decision ledger is rejected because that ledger is machine-local and disappears with the trail, which is exactly what the motivation must not do. Deriving `basedOn` from a run's skill-usage log is rejected because that log records the hash of the `SKILL.md` bytes a run opened, deliberately not the procedure hash, and says nothing about which supporting files were read.

## Testing Decisions

Good tests here assert what an operator can observe: what a row says, what the diff shows, what the install wrote into the skill, and what the shell prints. A test that recomputes an internal relation from private structures instead of reading the row is testing the wrong thing.

- **Relation** — a proposal for a name the store does not hold reads `new`; an identical procedure reads `same`; a base matching the installed hash reads `in-step`; a base matching nothing reads `moved`; a proposal with no base and an installed name reads `unbased`; the decision state is unchanged in every case. Prior art: the proposal-inventory tests in the skills package, which build trails on disk and read the projection.
- **Identity invariant** — for the same pair of bundles, "procedure diff is empty" and "procedure hashes are equal" agree: a pair differing only in frontmatter hashes equally and diffs empty, and a pair differing in one supporting file hashes differently and diffs non-empty. This is the one property worth writing as a property test.
- **Diff content** — a changed body line appears once, in the right file, with the right side; an added and a removed file are both named; a binary file is named with no body; an unchanged file is counted and not rendered; a bundle whose procedure is identical produces no patch and a sentence instead of an empty table. Prior art: the patch-parsing tests in the console's frontend suite and the merge-diff tests in the console package.
- **Metadata blocks** — a changed description appears as a field change; a provenance key present only on the installed side appears in "what the install writes" and never in the procedure diff; a hand-written extra key is preserved and shown as unchanged.
- **History** — the first install of a name appends no line; an install over an installed version appends exactly one line with the decision's time, the procedure hash prefix and the trail, so the first line a skill ever records describes its second version; a forced replacement by hand appends a `shell` line and keeps the earlier lines; a bundle that copies the installed history into itself appends one line and replays none of the copied ones; a first install whose bundle carries history keeps those lines and appends one for the version landed; a hand-written skill has no history and the surfaces say so rather than printing an empty label; a motivation longer than the cap is shortened and the install still succeeds; history past its budget is a `doctor` finding and never blocks an install; a body and its supporting files are byte-identical after an install apart from frontmatter; the procedure hash is unchanged by an append. Prior art: the provenance tests in the skills package, which assert unrelated keys and the body survive an install.
- **Never a second writer** — an unapproved hand edit to a skill produces no history line, still produces the drift finding, and the drift is reported on the revision read without changing the relation.
- **Failure and edges** — a missing bundle, an invalid bundle, a refused proposal already decided, a disabled installed entry, a store entry whose name is not a valid skill, and a diff read larger than the cap each produce the state the operator needs, never a blank pane and never a 404 for a proposal that exists.
- **Console** — the row names the relation, opening a revision shows the three blocks, the patch is fetched only when asked for, the existing renderer's layouts both work, and no editable control appears. The console's frontend tests already assert the read-only property of this modal, and that assertion is extended rather than replaced.
- **Shell parity** — `skills proposals --json` and the console send the same relation for the same trails; `skills show --json` carries history; the rendered and JSON shapes are asserted separately, so a rendering change cannot silently change a contract.

## Out of Scope

- **Forcing a replacement from the console.** The plan and approval paths stay force-free; a revision is decisive in the shell. Reopening that is a separate spec.
- **A revision browser.** No page lists a skill's versions, and no surface walks history beyond the lines the skill carries.
- **Deriving history from git.** The content history stays git's; no surface renders commits, and no mapping between a commit and a history line is claimed.
- **Editing history from the console.** History is written by an install and by nothing else.
- **Trials as a diff baseline.** A trial installs into a destination trail's candidate configuration; diffing a proposal against that candidate, or against another trail's frozen copy, is not covered — the baseline here is the repository store.
- **A three-way merge or conflict resolution.** `moved` informs; it never attempts to reconcile two revisions.
- **Retroactive history.** Proposals and skills that predate this spec get no backfilled lines; an absent history stays absent.
- **Nuc packs beyond carrying what the store holds.** A history line for a Nuc installation is named as the origin; nothing else about Nuc packaging changes.
- **An operator's own reason at approval.** The history line carries the proposal's argument; a separate operator note on approve is not introduced.
- **Authoring skills outside a proposal.** Writing a new procedure by hand keeps its current story: no history, and `doctor` reports edits to an installed one.

## Further Notes

**Why history lives in the skill and not in a side ledger.** The motivation for a change is produced on a machine and consumed by whoever reads the repository later, which may be another machine, another colony, or a person with no trail on disk. The provenance keys already took that position — they travel inside the file because the file is the portable unit — and history is the same argument over time instead of once. The decision record stays where 041 put it, beside the trail it decides; this spec adds a *summary* that survives the trail's deletion, not a second decision log.

**What the console shows in the example that motivated this spec.** A trail proposes `paseka-event-emit` while the store already holds it from an earlier, approved round. Nine files, four changed, five byte-identical, and the proposal's argument names the three statements that the runs contradicted. Today that reads as one body and an approval error; after this spec it reads as a revision, four changed files, five unchanged, the description change, the first history line the skill would ever record, and a plan that says the replacement is the shell's to make.

**Phases.** (0) The relation and its store facts, on the shell and the console row — the base everything else cites. (1) The procedure diff and the three metadata blocks behind a separate bounded read. (2) History in the skill: encoding, append, and the two shell surfaces. (3) The console's revision view composed from (0)–(2), reusing the existing patch renderer. Each phase is independently shippable, and (0) is useful alone: it makes the failure readable before any diff exists.

**Relationship to 041.** 041 decision 12 keeps a proposal's bytes in the trail's comb; this spec does not move them. 041 decision 17 keys decisions by the bundle's procedure hash; this spec uses that key as the revision identity rather than adding a second. 041 decision 32 keeps the store as the destination and a trail as a trial destination; this spec diffs only against the store. 041's post-implementation change C1 folded a bundle into one console row; a revision row keeps that fold and adds the relation to it.
