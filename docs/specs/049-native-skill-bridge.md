# Spec 049: Native client skill bridge

## Status

**(Draft)**
Split out of [spec 041](041-skills-layer.md) when its Phases 0–3 shipped. The design is 041's decision 24 carried forward unchanged; the one open item is where a bridged directory lives for a role that works in the repository root rather than in a worktree. Nothing in this spec is implemented.

## Problem Statement

Spec 041 delivers a role's skills to the model by advertising them in the system prompt, which works for every adapter including a script Bee and a client with no skill support. What it does not buy is the client's own skill handling: Cursor, OpenCode, and Claude Code each read skills from directories of their own, and a client that has discovered a skill can offer its own affordances — the skill's name in the client's skill list, a slash command, a way to run a bundled script. Paseka's store is not one of those directories, so a Bee running under such a client sees the procedure as prose and nothing more.

The obvious fixes both fail in Paseka's terms:

**Pointing the client at the store hands it every skill.** A client advertises every skill it discovers to every agent it starts, so the per-role gate — the reason the layer exists — disappears in one line of configuration. A procedure meant for the release role becomes available to the archivist.

**Copying into a client discovery directory is the same loss, with a file to maintain.** `.agents/skills/`, `.cursor/skills/`, `.claude/skills/` are read by the client, not by Paseka, so the colony can neither count, gate, nor verify what is in them, and the copy drifts from the store the moment either side changes.

The result today is a forced choice with no third option: a role either gets the gate and loses the client's native affordances, or gets the affordances and loses the gate. The advertisement made the first option workable but did not make it complete.

## Solution

For one run or one interactive session, the runtime materialises the resolved skill set as a discovery directory that contains **exactly** that set and nothing else, puts it where that run's client looks, and lets the client discover it.

The gate survives by construction rather than by policy: the directory is built from the same resolved set the advertisement is built from, so a skill the role does not list is not in it, a disabled skill is not in it, and an invalid skill stops the run instead of being silently missing. There is no second selection rule to get wrong, because there is no second selection.

The directory holds links to the trail's frozen copy, not to the live store, so a store edit after trail start cannot change what a running trail's client discovers — the same pinning the advertisement already has. It lives in the run's own data, so using a skill leaves no file in the Beekeeper's working tree and a trail purge removes it with everything else.

The advertisement stays. A skill delivered both ways is idempotent from the Bee's point of view: the model sees one entry per procedure and the client sees one discoverable skill per procedure. The bridge changes where the client discovers the set, not what the colony stores, approves, or counts.

The bridge is an adapter capability. A client that reads a discovery directory gets one; a client that does not is unaffected and keeps the advertisement it already had. The runtime owns what goes into the directory; the adapter owns where the client expects to find it.

## User Stories

1. As a Beekeeper, I want a skill my role lists to appear in my client's own skill list, so that the Bee can invoke it the way the client intends rather than only reading about it in the prompt.
2. As a Beekeeper, I want the bridged directory to contain exactly the resolved set and nothing else, so that native discovery cannot re-open a decision the role list closed.
3. As a Beekeeper, I want a skill my role does not list to be invisible to the client, so that "this procedure belongs to the release role only" stays true under native discovery.
4. As a Beekeeper, I want a disabled skill to be absent from the bridged directory, so that an emergency stop covers what the client can invoke, not only what the prompt advertises.
5. As a Beekeeper, I want an invalid skill to stop the run before the client starts, so that a broken procedure is never quietly the one thing missing from the client's list.
6. As a Beekeeper, I want the bridged directory to live in the run's data rather than in my working tree, so that using a skill does not dirty the repository or appear in a diff.
7. As a Beekeeper, I want a trail purge to remove the bridged directory with the rest of the run data, so that nothing accumulates beside the store.
8. As a Beekeeper, I want a crashed or killed run to leave no discovery directory behind, so that cleanup does not depend on the run finishing politely.
9. As a Beekeeper, I want the bridged skills to be the trail's frozen bytes, so that an edit to the store does not change what a running trail's client can invoke.
10. As a Beekeeper, I want a trail export to keep reporting what was offered and what was reached for, so that native use stays attributable rather than becoming invisible because the client did the loading.
11. As a Beekeeper, I want a role that works in a worktree to get the bridge first, so that the obvious location is used before the harder case is decided.
12. As a Beekeeper, I want the bridge to be opt-in, so that a colony that is happy with the advertisement keeps exactly today's behaviour.
13. As a Beekeeper, I want a client that cannot accept a bridged directory to say so loudly, so that a role does not silently receive less than its list declares.
14. As a Beekeeper, I want a skill's bundled `scripts/`, `references/`, and `assets/` to be reachable through the bridge, so that a procedure with a script is a unit for the client too.
15. As a Beekeeper, I want a resumed provider session to see the same bridged set it started with, so that resumption does not change what the Bee can invoke.
16. As a colony author, I want the bridged entries to be standard-shaped skill directories, so that a client reads them unchanged and a skill stays portable.
17. As a maintainer, I want the bridge to consume the same resolver output as the advertisement, so that the two cannot disagree about what a role was offered.
18. As a maintainer, I want resolution and delivery to stay where spec 041 put them, so that adding a client touches the adapter and nothing else.
19. As a maintainer, I want a client without native discovery to be unaffected except for the capability saying it is unavailable, so that the feature degrades to today's behaviour instead of failing a run.
20. As a maintainer, I want the bridge to be removable without touching the store, the ledger, or the proposal path, so that a client regression is a delivery change and not a data migration.

## Implementation Decisions

1. **The bridged set is the resolved set, computed once.** The directory is built from the same resolved references the advertisement is built from (spec 041, decision 7), in the same run, from the same frozen configuration. There is no second selection syntax, no second pattern expansion, and no second invalid handling: an entry the resolver dropped for being disabled is absent, and an entry that would fail validation has already stopped the run.

2. **The entries are the trail's frozen copy, not the live store.** Every link resolves inside the trail's configuration snapshot, so the pinning property spec 041 established for the advertisement holds for the client too. A store edit reaches a running trail only through spec 042's explicit configuration application, exactly as it does for the prompt.

3. **The directory lives in the run's data, never in the working tree.** It is runtime-owned ephemeral bridge data under the run's own directory, alongside the rendered prompts and run records; it is not the store, is not committed, and is removed with the run data. This is what makes cleanup a property of the run rather than a promise of the adapter, and it is what keeps a role that works in the repository root from leaving links in a Beekeeper's checkout.

4. **Links, with a copy fallback that still uses frozen bytes.** A link keeps the bridge cheap and keeps one skill body in one place; where a link cannot be created (cross-device, a platform that will not follow one, a client that resolves paths itself), the runtime copies the frozen entry instead. Either way the content is the frozen content, and the adapter is not asked to implement the fallback.

5. **The adapter owns where the client looks, the runtime owns what is there.** The launch request already carries the resolved references for delivery (spec 041, decision 7); the bridge adds an optional capability: the adapter names the directory its client discovers and receives its contents from the runtime. A client whose discovery is a directory follows that shape; a client configured with an explicit list of skill directories gets the same directory named in its own per-run configuration, which the adapter writes, because only the adapter knows the client's configuration format.

6. **The capability is optional and reports unavailability.** An adapter that has not implemented the bridge says so the same way the existing session-log resolver reports an unsupported provider. A colony with such an adapter runs on the advertisement alone and is not worse off than today.

7. **The bridge is opt-in, and the advertisement is not.** A colony that wants the client's own affordances enables the bridge; a colony that does not keeps the advertised set. The advertisement is never removed as a consequence of the bridge existing, because a bridged run still needs the names, descriptions, and paths that make a skill knowable to the model, and because a client that half-supports discovery must not lose the set entirely.

8. **Worktree first.** The location question has an obvious answer inside a worktree and a contested one in the repository root; the first ship therefore states where the bridged directory goes for a worktree run and refuses rather than guessing for a root run. A role that works in the root keeps the advertisement and is told why.

9. **Resume and re-attempt follow the advertisement's rules.** A resumed provider session keeps the directory it started with, and the runtime does not rebuild or re-inject it. A new attempt or a new session gets a directory built from the same frozen configuration, so the offered set is stable for the life of a run.

10. **Attribution maps the bridged path back to the skill.** A native tool call that names a file under the bridged directory is attributed to the skill behind the link, so spec 041's usage attribution keeps working when the client did the discovery. The path mapping is the runtime's, not the adapter's, because the runtime built the directory.

11. **Failure to materialise is a run failure, not a silent downgrade.** If the bridge is enabled, the client advertises support, and the directory cannot be built or placed, the run does not start with a smaller set. The same fail-fast rule that governs resolution governs delivery: a role either receives what its list declares or the run says why it cannot.

12. **The bridge does not touch the store, the ledger, the proposal path, or the role lists.** It is a delivery mechanism. Approval and activation keep the separation spec 041 established, and disabling a skill changes the bridge on the next run because it changes the resolved set, not because the bridge reads the ledger itself.

## Testing Decisions

Tests assert what an operator can observe: what the client could discover, what the Bee was offered, what a run recorded, and what cleanup left behind.

- **Gate** — a role listing two skills yields a directory holding those two and no other store entry; a role listing `*` yields every enabled skill; a role listing `paseka-*` yields the family; a role with no list and no default yields no directory at all.
- **Exclusion** — a disabled skill is absent from the directory while the run still starts; an invalid skill stops the run before the directory is built.
- **Pinning** — the directory's entries resolve to the trail's frozen copy; editing `.paseka/skills/` after trail start changes neither the directory nor its contents; a missing or corrupt snapshot blocks the run.
- **Location** — the directory is created under the run's data and nothing appears in the repository root or the worktree's tracked tree; a worktree run gets a usable directory; a root run on the shipped path refuses with a reason rather than guessing.
- **Isolation and cleanup** — a normal finish, a killed run, and a trail purge each leave no bridge directory behind; the store and ledger are byte-identical before and after a run.
- **Delivery coexistence** — the advertisement is unchanged by the bridge; a run with the bridge enabled and a run without it advertise the same names, descriptions, and mode-appropriate paths; a client with no bridge capability still receives the advertisement.
- **Bundled files** — a linked skill's `scripts/` file is reachable through the directory the client discovers.
- **Resume** — a resumed session keeps its first directory, and a store edit during the session changes neither the directory nor the advertised set.
- **Attribution** — a tool call naming a file under the bridged directory attributes the skill behind it, and a call naming an unrelated file attributes nothing; the used set is still the union with load records, deduplicated by name.
- **Opt-in** — a colony that has not enabled the bridge produces no directory and behaves exactly as before.

Prior art in-repo: the resolver and advertisement tests from spec 041, the adapter capability tests for the session-log resolver, the run-directory and run-record tests, the trail-configuration pinning and missing-content tests, the runtime dispatch tests over rendered prompts, and the run-directory cleanup tests.

## Out of Scope

- Writing skills into a client discovery directory **inside the repository** (`.agents/skills/`, `.cursor/skills/`, `.claude/skills/`) or into a client's home directory. The bridge lives in run data precisely so that it does not become a second, ungated store.
- Making the store itself a client discovery path. Spec 041 decision 1 stands: the store is not a client directory, and a client that reads it would advertise the whole store.
- A machine-wide or user-level bridge that outlives a run.
- Changing what the store holds, how a skill is validated, or how a proposal is approved.
- A per-skill bridge toggle. The bridge follows the resolved set as a whole.
- Client-specific command affordances that Paseka would generate (slash commands, prompt wrappers). The client derives those from what it discovers.
- Streaming or progressive discovery during a run. The set is fixed when the run or session is created.
- Any change to the advertisement's shape or to the `path` / `command` delivery modes.

## Further Notes

- **The open question, restated.** Every client that supports skills reads a directory of them, and OpenCode additionally accepts an explicit list of directories in its configuration, so the mechanism is small. What is not settled is where the bridged directory lives for a role that works in the repository root: run data is clean but may be outside the tree the client treats as its workspace, while a directory inside the workspace appears in the Beekeeper's checkout. Worktrees answer it, because there the workspace is already a disposable copy. A colony that cannot say "where do the links go, and who removes them" should not place them in a working tree, which is why the first ship is worktree-first and refuses elsewhere.
- **Why this is its own spec.** Spec 041's definitions of done for Phases 0–3 are shipped and observable in the shell, the runtime, the export, and the console. The bridge has a different definition of done, a different risk surface (it is the one mechanism that could widen the gate), and an unresolved location decision. Keeping it inside an Implemented spec would either hold the record open indefinitely or make the status untrue.
- **The advertisement remains the compatibility surface.** In either delivery mode a Bee reading a Paseka-advertised skill is reading a file the client would also have accepted; a bridge only adds the client's own discovery on top. A skill offered twice is idempotent from the Bee's point of view, and a skill offered natively is one the runtime also counted.
- **The gate is the property to protect in review.** A bridge that links the resolved set is a delivery optimization. A bridge that links the store, or that writes into a directory the store is not, is the escalation spec 041 warns about: it would let a client advertise a procedure to a role whose list does not carry it. Any future change that makes the bridged directory's contents differ from the resolved set should be read as a new design, not a tweak.
