# Spec 041: Skills layer

## Status

**(Draft)**
The colony owns a skill store, Bees declare which skills they may use, the runtime resolves and delivers them, adapters report what was used, and the Archivist Bee can propose a new skill that a Beekeeper approves. Design settled in review; awaiting approval before implementation.

Scope of this spec is the whole layer — store, selection, delivery, attribution, proposal — described as four phases in Further Notes. Phase 1 (store, selection, delivery, run record) is the implementable core; Phases 2–4 (attribution and export, proposal and approval, native client bridge) build on it.

## Problem Statement

A Bee that does generic work well still does this colony's work badly, because the colony's procedures live only in a Beekeeper's head and in prompt templates. The Beekeeper writes a procedure down once, in the best place available — the role's prompt template — and from then on it is unconditional, unconditional per role, and grows without bound. There is no way to say "this procedure applies to the release task and to nothing else", no way to hand the same procedure to two roles, and no way to know which procedure a given run actually used.

The industry has converged on a format for exactly this. The Agent Skills specification defines a portable directory — a `SKILL.md` with YAML frontmatter plus optional `scripts/`, `references/`, and `assets/` — and the major clients already read it. Cursor discovers skills from `.agents/skills/`, `.cursor/skills/`, `~/.agents/skills/`, and `~/.cursor/skills/`. OpenCode discovers them from `.opencode/skills/`, `~/.config/opencode/skills/`, `.claude/skills/`, and `.agents/skills/`, and additionally accepts an explicit list of skill directories in its config. Claude Code reads `.claude/skills/` and `~/.claude/skills/`. The format is portable, the clients are ready, and Paseka has no story for it.

Every existing workaround is bad in a different way, and all of them fail in Paseka's terms rather than the client's.

**A skill dropped into a client directory is invisible to the colony.** It is loaded by the client, so the Bee may use it, but the colony never learns of it. The Bee's config does not list it, so no Beekeeper reviewing a change can see that this role now carries a procedure. The colony cannot answer "which skills did this run have", so a trail export cannot show it, and the trail's history cannot be compared against the skills that were on offer. The whole point of a colony is that its configuration is reviewable, and this configuration is not in the colony.

**A skill dropped into a client directory cannot be gated.** The client advertises every skill it discovers to every agent it starts. The one thing Paseka needs most — that a procedure reaches the Bees that should have it and no others — is exactly the thing the client cannot express. A private procedure and a shared procedure are indistinguishable to the client.

**Paseka's own configuration is not a discovery path for any client.** A Bee that runs under Paseka is an external CLI in a working directory, and that CLI looks for skills in its own conventions. The colony's manifest, the Bee configs, and the prompt templates are invisible to it. So a skill that *is* under the Beekeeper's control — in the repository, in a diff, in the code review — is a skill the Bee cannot see, and a skill the Bee *can* see is one the Beekeeper cannot review. The two sets do not intersect, and Paseka currently offers no way to make them intersect.

**Nothing records what a Bee was given.** A run's record holds the prompt, the system prompt, the model, and the token usage. It does not hold the reusable procedure packs that were on offer, and certainly not whether any of them was used. A Beekeeper auditing a trail cannot tell the difference between a Bee that followed the release procedure and one that improvised, because the release procedure is not in the trail at all.

**A repeated failure has nowhere to become a procedure.** The colony already has an Archivist Bee whose whole job is to read a finished trail and write down what should be remembered — the deferred work, the review notes that were never applied, the merge lag. It sees the recurring shape of a colony's friction constantly. It has nowhere to put a procedure when it sees one, because the store does not exist, and because a procedure that a Bee wrote must not be able to install itself into the Bees that will later obey it.

## Solution

Give the colony a skill store it owns, and make every step of a skill's life a colony step.

**The store is the colony's.** A skill is a directory in the Agent Skills format, held in one of two roots: a shared root inside the repository, version-controlled and reviewed like any other colony configuration, and a machine-local root under the colony's home configuration, private to that machine and never committed. The store is spec-shaped, so a skill written for Paseka is readable by any client that speaks the standard, and a skill written elsewhere can be dropped in.

**Selection is the colony's.** A Bee's configuration lists the skills it may use. The list is a gate, not a hint: a skill that is not listed for a Bee does not reach that Bee, no matter what any client would find on disk. A Bee that lists nothing gets nothing. A Bee's list replaces the colony-wide default rather than adding to it, because "remove this procedure from this one role" has to be expressible.

**Delivery is the runtime's, the mechanism is the adapter's.** The runtime resolves the effective skill set for a run and hands it to the adapter. The adapter knows how its own CLI takes instructions and is responsible for making them available. The first delivery mechanism is deliberately universal: the runtime renders the skill metadata — name, description, and where the file is — into the run's system prompt, which is the same shape the clients themselves advertise, and which works for every adapter including the ones with no skill support at all. The client-native mechanisms, where a client can be pointed at a directory of skills directly, are a later phase and are additive.

**Use is recorded.** The runtime records which skills a run was offered. Adapters that can see what their CLI actually did report which skills were used, and the trail export carries both — so a Beekeeper reading a trail can see the procedures that were on offer and the ones the Bee reached for.

**A proposal is a proposal until a human approves it.** The Archivist Bee can read a finished trail, recognise a procedure worth keeping, and write it out as a proposal artifact. It publishes a discovery saying a skill was proposed. It cannot write into the store, and nothing installs a proposed skill on the colony's own initiative. The Beekeeper reviews the proposal, and the runtime — never the Bee — validates and installs it. Approval makes a skill *available*; it does not make it *active*, because activation is the per-Bee list, which is reviewed in the repository like everything else.

The properties that make this safe rather than merely convenient:

- **Approval and activation are separate.** A skill the Beekeeper approved sits in the store until a Bee's configuration lists it. The blast radius of a bad skill is the set of Bees that opted in, and that set is visible in a diff.
- **The proposer never writes the store.** The Bee writes a proposal artifact into the trail's comb; the runtime writes the store. A Bee that gets confused, or manipulated, cannot install instructions for its successors.
- **A human approves.** There is no auto-approval path, for the same reason there is no path by which a run promotes itself.
- **Provenance is durable.** An installed skill records where it came from — the trail that proposed it, who approved it, and when — so an approved-but-surprising skill is explainable later.

## User Stories

1. As a Beekeeper, I want to write a procedure once as a skill and use it in any Bee, so that the same knowledge is not re-explained per role.
2. As a Beekeeper, I want a skill to live in the repository, so that it is version-controlled, reviewable, and shared with my team.
3. As a Beekeeper, I want a skill to live in my machine's colony configuration, so that a private or machine-specific procedure never has to be committed.
4. As a Beekeeper, I want a Bee's configuration to list the skills it may use, so that the procedures a role carries are visible in the diff that changes it.
5. As a Beekeeper, I want a Bee that lists no skills to receive none, so that a role never silently inherits procedures it did not ask for.
6. As a Beekeeper, I want to remove a procedure from one role while leaving it in others, so that a skill is not all-or-nothing for the colony.
7. As a Beekeeper, I want a colony-wide default skill list that roles inherit unless they override it, so that a common procedure is declared once.
8. As a Beekeeper, I want to opt a role out of the colony-wide default explicitly, so that the difference between "inherits" and "declares nothing" is readable.
9. As a Beekeeper, I want an experimental procedure to live outside the repository and be referenced explicitly, so that I can try it without dirtying the tree my team reviews.
10. As a Beekeeper, I want to move an experimental procedure into the repository when it proves out, so that it becomes shared without being rewritten.
11. As a Beekeeper, I want to pin a reference to one specific tier, so that a private experiment never quietly shadows the shared procedure of the same name.
12. As a Beekeeper, I want a shared procedure to win over a private one of the same name when a role does not pin a tier, so that the reviewed version is the one that normally runs.
13. As a Beekeeper, I want one role to be able to list a family of skills by pattern, so that a growing set of related procedures is not re-listed per role.
14. As a Beekeeper, I want a malformed skill to be reported with its name and the reason, so that a typo in frontmatter does not silently disable a procedure.
15. As a Beekeeper, I want `doctor` to report a role that references a skill that does not exist, so that a rename cannot leave a role silently skill-less.
16. As a Beekeeper, I want `doctor` to report a skill that exists but does not satisfy the format, so that a broken procedure is visible before a Bee needs it.
17. As a Beekeeper, I want to list every skill the colony can see with its tier, description, and validity, so that I know what is on offer without reading directories.
18. As a Beekeeper, I want to see one skill's resolved file location and content, so that I can read exactly what a Bee would be told.
19. As a Beekeeper, I want a run's record to name the skills that were on offer, so that a trail tells me what procedures the Bee had.
20. As a Beekeeper, I want a run's system prompt on disk to contain the advertised skill block, so that what the Bee was told is auditable, not reconstructed.
21. As a Beekeeper, I want a trail export to list the skills each run was offered, so that an offline report is self-describing.
22. As a Beekeeper, I want a trail export to list the skills each run actually used, so that I can see which procedures earned their place.
23. As a Beekeeper, I want an export to carry the body of a used skill, so that the exported trail still explains itself months later.
24. As a Beekeeper, I want the export to say plainly that usage could not be determined for a run, so that absence of evidence is not read as absence of use.
25. As a Beekeeper, I want the Archivist Bee to be able to propose a skill after reading a finished trail, so that a recurring failure becomes a procedure instead of a memory.
26. As a Beekeeper, I want the proposal to cite the runs, files, and events that motivated it, so that I can judge the evidence rather than take the claim on faith.
27. As a Beekeeper, I want to review a proposed skill in the console or the shell, so that approving it is a deliberate act.
28. As a Beekeeper, I want to approve a proposed skill into the shared store, so that it becomes part of the colony.
29. As a Beekeeper, I want to approve a proposed skill into my private store instead, so that I can trial it before sharing it.
30. As a Beekeeper, I want approval to be refused by default and to require my action, so that a Bee can never install instructions for its successors by itself.
31. As a Beekeeper, I want an approved skill to stay inert until a role lists it, so that approving is not the same as deploying.
32. As a Beekeeper, I want an approved skill to record which trail proposed it and that I approved it, so that a surprising skill is explainable later.
33. As a Beekeeper, I want to be told which roles reference a skill before I remove it, so that a removal does not silently strip a procedure from a role.
34. As a Beekeeper, I want to disable a skill without deleting it, so that I can neutralise a bad procedure and keep its history.
35. As a Beekeeper, I want the console to show that a skill was proposed rather than hand-written, so that I read it with the right suspicion.
36. As a Beekeeper, I want a skill to be usable by a script Bee, so that a non-LLM step can read the same procedure the LLM steps read.
37. As a Beekeeper, I want a script Bee to receive the resolved skill set as data, so that it can use it without knowing Paseka's internals.
38. As a Beekeeper, I want the skill block to cost roughly the same whether there are two skills or twenty, so that adding procedures does not quietly inflate every prompt.
39. As a Beekeeper, I want skill bodies loaded only when a Bee actually needs one, so that a large procedure does not sit in every run's context.
40. As a Beekeeper, I want a Bee's use of a skill to be visible on the run, so that I can tell a procedure that works from one that is never reached for.
41. As a Beekeeper, I want an interactive session to report skill use the same way an unattended run does, so that HITL and AFK trails are comparable.
42. As a Beekeeper, I want a run's advertised skills to be recorded even when nothing used them, so that "offered and unused" is a finding rather than a silence.
43. As a colony author, I want skills to be portable to another client, so that a procedure I wrote is not trapped inside Paseka.
44. As a colony author, I want a skill written for another client to be usable in Paseka, so that the standard is not a one-way door.
45. As a colony author, I want a skill directory to carry its supporting files, so that a procedure with a script or a reference template is a unit.
46. As a colony author, I want a skill's name to come from its own metadata rather than its folder, so that a rename is visible and errors are legible.
47. As a maintainer, I want one component to resolve skills, so that two call sites cannot disagree about what a Bee was offered.
48. As a maintainer, I want resolution to be a pure function of colony state, so that a run's skill set can be recomputed and compared.
49. As a maintainer, I want the adapter boundary to own delivery, so that adding a client does not require touching the resolver.
50. As a maintainer, I want skill usage to be an optional adapter capability, so that an adapter without the knowledge degrades to "not available" instead of failing a run.
51. As a maintainer, I want usage attribution to reuse the existing tool-call reading, so that a new provider does not need a second transcript parser.
52. As a maintainer, I want a Bee to be unable to write into the store, so that the store's only writer is the runtime.
53. As a maintainer, I want installation to validate the format before copying a single byte, so that a broken proposal never lands half-installed.
54. As a maintainer, I want installation to refuse to overwrite an existing skill without an explicit override, so that a proposal cannot silently replace a reviewed procedure.
55. As a maintainer, I want the proposal path to reuse the artifact protocol, so that a proposed skill is stored and announced the way every other Bee output is.
56. As a maintainer, I want a skill proposal to be its own discovery kind, so that it is distinguishable from a note that merely mentions skills.
57. As a maintainer, I want a role's completion contract to be able to differ per intent, so that a role with two kinds of output can require the right one for each.
58. As a maintainer, I want the store to be a plain directory of plain files, so that it survives every tool in the chain — git, diff, an editor, a file browser.
59. As a maintainer, I want the skill block rendered in one place, so that every adapter advertises skills identically.
60. As a maintainer, I want the store to be excluded from the trail's run tree, so that cleanup of run data does not touch it.
61. As a Beekeeper, I want a single `paseka skills` command group, so that every operation on a procedure is one place.
62. As a Beekeeper, I want `paseka skills list` to show every skill the colony can see with its tier and its state, so that I know what is on offer.
63. As a Beekeeper, I want the listing to name the roles that carry each skill, so that I can see the gate at a glance.
64. As a Beekeeper, I want a skill that exists in both roots to be marked as shadowed, so that I am not surprised by which definition runs.
65. As a Beekeeper, I want an invalid skill listed with the reason it is invalid, so that a broken procedure is visible before a Bee needs it.
66. As a Beekeeper, I want `paseka skills show` to print a skill's metadata and body, so that I can read exactly what a Bee would be told.
67. As a Beekeeper, I want to print only a skill's resolved path, so that a script can consume it.
68. As a Beekeeper, I want `paseka skills validate` to exit non-zero when something is invalid, so that it can guard a commit.
69. As a Beekeeper, I want to list the proposals the trails hold, so that I can find what to review without knowing which trail to open.
70. As a Beekeeper, I want a proposal to show as pending, installed, or refused, so that I do not review the same one twice.
71. As a Beekeeper, I want to approve a proposal into the repository by default, so that the procedure becomes shared.
72. As a Beekeeper, I want to approve a proposal into my machine store instead, so that I can trial it privately.
73. As a Beekeeper, I want approval to tell me the repository now has an uncommitted change, so that I know the team does not have the skill yet.
74. As a Beekeeper, I want to refuse a proposal and have that refusal remembered, so that the same proposal is not offered again.
75. As a Beekeeper, I want to move a skill from my machine into the repository, so that a proven procedure becomes shared.
76. As a Beekeeper, I want to move a skill out of the repository and back to my machine, so that I can take a procedure private without rewriting it.
77. As a Beekeeper, I want a move to tell me which roles referenced the skill, so that I know what the move affects.
78. As a Beekeeper, I want the command to rewrite those references for me on request, so that a move does not silently strip a procedure from a role.
79. As a Beekeeper, I want every store change previewable before it happens, so that a change is never a surprise.
80. As a Beekeeper, I want removal to refuse while a role still uses the skill, so that I cannot accidentally strip a procedure.
81. As a Beekeeper, I want to disable a skill without deleting it, so that I can stop a bad procedure at once and keep its history.
82. As a Beekeeper, I want the same reference syntax in the shell as in a role's configuration, so that I do not learn two dialects.

## Implementation Decisions

1. **Two roots, and the project root shadows the machine root.** A skill lives in one of two directories: the shared root inside the colony configuration in the repository, and the private root under the machine-local home configuration for the colony. A reference with no tier prefix resolves in the shared root first and in the private root second, so a reviewed, committed procedure is the one that normally runs. This mirrors the precedence the Agent Skills standard already establishes between project-level and user-level skills, so a colony and a client agree about which definition of a name is in force.

   The private root is per colony and per machine, matching the rest of the home configuration. It is not a global skill store: a procedure a Beekeeper wrote for one colony is not automatically offered to another.

2. **Tiers are explicit in the reference, not only in the path.** A reference may name a tier: `shared:` pins the repository root, `local:` pins the private root, and an unqualified name resolves by the shadowing rule above. Without the explicit form, a private experiment named like a committed procedure would be invisible rather than wrong — the shared one would simply run, and the Beekeeper would believe they were testing something new. With it, the pin makes the intent unambiguous.

   The two explicit tiers are named `shared` and `local` because those describe what the Beekeeper sees — a shared procedure and a local one — rather than where a file happens to sit.

3. **A role's list replaces the colony default; it does not extend it.** The colony manifest carries an optional default skill list, and a Bee's configuration carries its own. When a Bee declares a list, that list is the whole set for that Bee; the colony default does not apply. Merging would make "this procedure must not reach this role" unexpressible, and the per-role gate is the property the whole layer exists to provide.

   The absence case is therefore tri-state and is treated as such: a Bee that omits the field inherits the colony default, a Bee that declares an empty list opts out of it explicitly, and a Bee that declares a non-empty list uses exactly that list. An explicit empty list is easy to lose in a diff, so `doctor` reports it as a likely mistake rather than passing it silently.

4. **No declaration anywhere means no skills.** With neither a colony default nor a role list, a Bee receives nothing. A procedure is executable content with a context cost, so the default is the safe one, and the cost of opting in is one line in a reviewed file.

5. **The store holds spec-shaped directories, one per skill.** A skill is a directory named for the skill, holding a `SKILL.md` with YAML frontmatter and a Markdown body, and optionally `scripts/`, `references/`, and `assets/` subdirectories holding its supporting files. Nothing in the format is Paseka-specific, so a skill moves to another client unchanged, and a skill authored elsewhere moves in unchanged.

   The store is flat: one directory per skill, no category folders. Clients differ on whether they recurse into a skills root — Cursor does, OpenCode does — but a flat store is read identically by every one of them, and a category hierarchy is a naming convention, not a capability.

6. **Validation is a load-time gate, and it is the same validation on every path.** A skill is valid when its name is present, matches the standard's shape (lowercase letters, digits, and hyphens, no leading or trailing or doubled hyphen, at most 64 characters), equals the name of the directory that holds it, and carries a non-empty description within the standard's length bound. A skill that fails validation is not offered; it is reported with its name and the reason.

   The same validation runs when the store is scanned, when a proposal is approved, and when `doctor` runs. Three copies of these rules would drift, so there is one rule set and one report shape.

7. **Resolution belongs to the runtime; delivery belongs to the adapter.** The runtime computes the effective skill set for a run — reading the two roots, applying the role's list, resolving references, validating, and applying the budget — and hands the result to the adapter. The adapter owns the question "how does my CLI receive these", because only the adapter knows the CLI.

   The seam is a small set of skill references carried on the launch request, plus the rendered advertisement the adapter may deliver. Adapters that speak a client's native skill mechanism use the references; adapters that do not fall back to the rendered advertisement. Adding a client touches the adapter and nothing else.

8. **The first delivery mechanism is a system-prompt advertisement.** The runtime renders the resolved skills as a block containing each skill's name, description, and absolute path to its `SKILL.md`, and appends that block to the run's system prompt. This is the same shape the clients themselves use to advertise skills to a model, and it is the only mechanism that works for every adapter, including a script Bee and a client with no skill support.

   The advertisement carries metadata only. Bodies are never inlined: the standard's progressive disclosure exists so that a procedure costs a small fixed amount until it is needed, and a runtime that inlined bodies would destroy that property. The Bee reads the file when the task calls for it.

   The block is rendered in exactly one place so that every adapter advertises identically, and it is written to the run's system-prompt file so that what the Bee was told is auditable rather than reconstructed.

9. **The run record carries what was offered.** The run's request record gains the names of the resolved skills, alongside the model and the model alias that are already recorded there for the same reason: the run's configuration is fixed at dispatch and must stay readable after the configuration changes. The advertisement itself is separately preserved in the run's system-prompt file.

10. **Usage is an optional adapter capability, mirroring the existing session-log capability.** An adapter that can read its client's transcript may report which skills the run used. The result is a list of uses, each with a skill name and the evidence for it — the client's own skill invocation, a read of the skill's file, or a run of a bundled script. A capability that is not implemented reports that plainly, in the same way the existing session-log resolver reports an unsupported provider, so a missing measurement is visible rather than read as a negative result.

    Attribution reuses the tool-call reading that already exists. A generic pass matches the paths in tool-call arguments against the resolved skill directories, which attributes a use for any client whose tool calls are already being read, without a client-specific rule. A client-specific refinement may be added later; it does not change the shape.

11. **The export carries both sides.** A new export section lists, per run, the skills that were offered and the skills that were used, and includes the body of each used skill so that the exported trail still explains itself. A run whose usage could not be determined says so in the same place, using the capability's own omission reason.

12. **A proposal is a bundle of files in the trail's comb, and a discovery of its own kind.** The Archivist Bee writes the proposed skill — its `SKILL.md` and any supporting files — into the trail's artifact comb, next to the report it already writes, and publishes a discovery of a dedicated kind carrying the skill's name, description, the reason, and the comb reference to the bundle.

    The comb is used because it already is where a Bee's output goes, it is already announced and visible in the console, and it is already part of the trail's history. The proposal travels with the trail that motivated it, and deleting the trail deletes the proposal.

    A dedicated kind rather than a general note is required because a proposal has a machine-readable shape and a lifecycle — it can be approved — and because the role's completion contract has to be able to demand it.

13. **A role's completion contract can differ per intent.** The Archivist Bee has two output kinds after this change — a note and a proposal — and the contract that checks the run's events must demand the right one for the intent that ran. The contract therefore accepts an optional per-intent override, with the role's own contract as the default. This is a schema extension to the Bee configuration, not a special case in the runtime, because the next role with two kinds of output will need it too.

14. **Only the runtime writes the store.** A Bee writes a proposal into the comb and publishes a discovery. Nothing else writes into either root. Installation is a runtime operation, invoked by the Beekeeper through the shell or the console, and it validates before it copies.

15. **Approval is a human act with no automatic path.** There is no policy, flag, or colony setting by which a proposal installs itself, and no path by which a run promotes itself. The argument is the one the whole layer is built on: a skill is instructions that later Bees obey, so a path from a Bee's output to the store is a path from a Bee to the control plane. The Beekeeper is a first-class participant in this colony, and this is the decision they are entitled to make.

16. **Approval installs; it does not activate.** An approved skill is available. It reaches a Bee only when that Bee's configuration lists it. This keeps the blast radius of an approved skill inside the repository diff that a Beekeeper reviews, and it means an approval can be made before anyone has decided who should carry the procedure.

17. **Installation is validated, refuses to overwrite, and records provenance.** The installer runs the same validation as a store scan, refuses to replace an existing skill without an explicit override, and writes a provenance record into the skill's own metadata: the trail that proposed it, the role that proposed it, the approval time, and a content hash. The metadata is string-to-string, as the standard requires, so provenance does not make the skill non-portable.

    The content hash also makes the ledger meaningful: two proposals of the same name with different content are distinguishable after the fact.

18. **Moving between roots is one operation in both directions, and it does not rewrite references by default.** A skill can be relocated from the machine root into the repository and back out of it, and the two directions are the same operation with a different destination rather than two different commands with two different vocabularies. The move validates before it moves, and reports which role configurations reference the skill so the Beekeeper can decide. Rewriting the references automatically is available behind an explicit flag, because a silent rewrite of role configuration is the kind of change that belongs to a person, not to a move command.

19. **The ledger is a file beside the store, not a directory inside it.** Each root keeps a small index of what it holds and where each skill came from. It lives beside the skills directory rather than inside it, so that a directory under the store is always a skill and never a bookkeeping file that a client might try to load.

20. **Removal and neutralisation are separate.** A skill can be removed from a store, and it can be disabled without being removed. Removal reports the roles that reference it before it happens, so a removal cannot silently strip a procedure from a role. A disabled skill stays in the store and in the ledger, with its history, and is simply never offered.

21. **The store is a plain directory of plain files, outside every ephemeral area.** It is not inside a trail's run tree, not inside a worktree, and not inside any cache. Trail cleanup, worktree cleanup, and cache purging leave it alone. It is plain files in a repository, so git, diff, an editor, and a file browser all work on it without the colony knowing.

22. **A script Bee receives the resolved skills as data.** The script adapter is given the skill set as a manifest — the same names, descriptions, and locations the advertisement carries — through the process environment the script adapter already passes, and the paths are absolute so the script does not need to know where it was started from. A script Bee that ignores the manifest is unaffected.

23. **Budgets are explicit and small.** A role may not be offered more skills than a fixed limit, a skill file may not exceed a size limit, and the total size of a store is bounded. The reason is context, not disk: the standard prices an advertised skill at a small number of tokens, and that price is only acceptable while the count is bounded. Exceeding a limit is reported, and the offending reference is dropped rather than the run failing.

24. **The pattern form is a name pattern, matched against the skill's own name.** A role may list a family of skills by pattern, so a growing set of related procedures is not re-listed per role. Patterns match the name declared in the skill's metadata. A pattern that matches nothing is reported, so a typo in a pattern is visible rather than a silently smaller set.

25. **The native client bridge is a later phase, and its shape is already known.** Cursor, OpenCode, and Claude Code each discover skills from client-specific directories, and OpenCode additionally accepts an explicit list of skill directories in its configuration. A native bridge would make the selected set discoverable by the client itself, which buys the client's own skill invocation and command affordances, and costs the colony the gate unless the bridged directory contains only the selected skills. The bridge is therefore a directory of links containing exactly the resolved set, placed where the client looks.

    It is deferred because the advertisement already delivers the set to every client, including those with no skill support, and because the bridge has to answer a question the colony cannot yet answer well: where to put a directory of links in a role's working directory without leaving files behind in the Beekeeper's repository. In a worktree the answer is obvious; in the repository root it is not.

26. **The Archivist's skill intent is a new intent, triggered by hand.** The role gains an intent for proposing a skill, discovered the way a role's intents are discovered, with its own mission guidance. It is run deliberately, against a named trail, rather than automatically on every finished trail: a proposal needs a human to look at it anyway, and a colony that proposed a skill after every flight would train its Beekeeper to ignore proposals.

    The intent changes what the role is allowed to do. Under the note intent the role writes one report and publishes one discovery; under the skill intent it writes a proposal bundle and publishes a proposal discovery. The role's rules state that it writes only inside the comb, and never into the store, in both cases.

27. **One command group, `paseka skills`, with one verb per thing a Beekeeper does with a procedure.** The group is `list`, `show`, `validate`, `proposals`, `approve`, `reject`, `move`, `remove`, `disable`, `enable`. The group is a namespace, not an action: invoking it bare prints its help rather than guessing which of the ten the Beekeeper meant, because six of them change the store and picking the wrong one is not a harmless mistake.

28. **The reference syntax is the same in the shell as in a role's configuration.** A reference may be a bare name, a tier-prefixed name, or a pattern, and it means the same thing wherever it is written. One syntax and one resolver, so a Beekeeper does not learn a second dialect in the shell, and so a command cannot disagree with the runtime about what a name refers to.

29. **The tier in a reference names the store the command acts on.** For a read it names the definition to read; for a refusal or a disable it names the ledger that records the decision; for a move it is the source. One rule with three readings, and each command states which reading it uses. The rule matters most for the decisions: a refusal recorded in the shared ledger is the team's, and a refusal recorded in the machine ledger is mine — and both are one word apart in the command line.

30. **Listing is a table whose state column is a verdict.** The columns are name, tier, state, roles, description. The state is one of `ok`, `invalid`, `shadowed`, or `disabled`: a skill present in both roots is listed twice and the shadowed one says so, and an invalid one carries the reason. The roles column names the roles whose configuration carries the skill — the question a gate exists to answer, asked first and in the same row as the skill it gates.

31. **Reading one skill is a different command from reading the store.** `show` resolves a single reference and prints its metadata, its provenance, and its body; `list` is the inventory. `show` can also print only the resolved path, so a script can consume the result without parsing prose.

32. **Validation is a command that can fail a pipeline.** `validate` checks one reference or the whole store and exits non-zero when something is invalid, so it can be a pre-commit hook. Its verdict is the same verdict the store scan and the approval path produce, because it is the same rule set rather than a second implementation.

33. **A proposal is found before it is approved.** `proposals` lists the proposals the trails hold, with the trail each came from, its state — pending, installed, or refused — and when it was proposed. Without it, a Beekeeper would have to know which trail to open to discover that a proposal was waiting, which is exactly the knowledge the proposal was supposed to produce.

34. **Approval names one proposal and installs it in one place.** `approve` takes a reference and, when a name was proposed on more than one trail, a trail to disambiguate; an ambiguous name without a trail is an error that names the candidates rather than a guess. The destination defaults to the shared root and can be pointed at the machine root, because both are legitimate: the first shares the procedure, the second trials it.

35. **Approval reports the repository consequence and never commits.** Installing into the shared root leaves an untracked file in the working tree, and the command says so, names the path, and says that the commit is still the Beekeeper's. The colony has no path that writes a commit on the Beekeeper's behalf, for the same reason it has no path that approves a proposal by itself.

36. **A refused proposal is recorded, not merely not-approved.** `reject` records the refusal together with the trail it came from and an optional reason, so the same proposal is not offered again and the trail's history shows that it was considered rather than ignored. The refusal lives in the ledger named by the tier, on the same rule as a disable.

37. **Move relocates, and copies only when asked.** `move` removes the source; `--keep-source` copies it. The destination must not already hold a skill of that name unless the Beekeeper forces it, because a move that silently replaces a reviewed procedure is the failure this whole layer is designed to avoid.

38. **A move names the roles it affects, and rewriting them is opt-in.** The command reports every role whose configuration references the moved skill, because a move changes what those roles resolve to. `--rewrite-refs` rewrites those references into the explicit destination form — a change to role configuration, therefore opt-in and previewable — and without it the references are left alone and the affected roles are reported, so a move never silently strips a procedure from a role.

39. **A move names its repository consequence.** Moving a skill into the shared root adds an untracked file; moving one out removes a tracked file. The command says which happened and that the change is not committed. A move that is not committed is a local fact, and the command says so rather than implying the team now has the skill.

40. **Removal and neutralisation stay separate commands, because their consequences differ.** `remove` deletes a skill and refuses while roles still reference it unless forced; `disable` keeps the skill, keeps its history, and records that it is not to be offered. Both record in the ledger named by the tier, so a machine-local stop and a team-wide stop are both one word apart.

41. **Every mutating command can be previewed.** `approve`, `reject`, `move`, and `remove` accept a dry run and print the plan — what would be written, what would be deleted, which role configurations would change — without touching the store. The store is a reviewed artifact, so a command that changes it must be able to show its intent first; a command that can only be run for real is a command whose mistakes are only visible in a diff.

42. **The read commands speak JSON on request; the mutating commands speak in prose.** `list`, `show`, and `proposals` emit a machine contract on request, for a script or another tool. The mutating commands are read by a person deciding something, and a person's reading of "installed into the shared root, working tree has one new file" should not be a JSON document.

## Testing Decisions

Good tests here assert what an operator can observe: what a Bee was offered, what it was told, what a run recorded, and what a Beekeeper can approve. A test that reaches into the resolver's internals is testing the wrong thing.

- **Resolution** — a role with no list and no colony default receives nothing; a role with a list receives exactly that list; a role that declares a list replaces the colony default rather than extending it; an explicit empty list opts out; a reference to a missing skill is reported and dropped, not fatal; a pattern that matches nothing is reported; a limit breach drops the offending reference and reports it.
- **Tiers** — an unqualified name resolves to the shared root when both roots hold it; `local:` resolves to the private root even when a shared skill of the same name exists; `shared:` never falls back to the private root; a skill present only in the private root is found by an unqualified name.
- **Validation** — a skill whose directory name differs from its metadata name is rejected; a name with an uppercase letter, a leading hyphen, a doubled hyphen, or excessive length is rejected; a missing or empty description is rejected; a valid skill of the same shape is accepted. The same rules must produce the same verdict on scan, on approval, and in `doctor`.
- **Delivery** — the rendered advertisement contains each resolved skill's name, description, and location, and contains no skill body; the advertisement is written into the run's system-prompt file; a run with no skills produces no advertisement; the advertisement is identical regardless of which adapter consumes it.
- **Run record** — the run's request record names the skills that were offered; the names survive a later change to the role's configuration.
- **Usage attribution** — a transcript containing a read of a skill file attributes that skill; a transcript containing no reference to a skill attributes none; a run whose adapter has no usage capability records an explicit "not available" reason rather than an empty result; usage attributed for one run does not leak into another.
- **Proposal** — the Archivist's skill intent writes a bundle into the comb and publishes a proposal discovery; the note intent still writes its report and publishes its note; the role's completion contract demands the note for the note intent and the proposal for the skill intent; a run that publishes neither fails its contract.
- **Approval** — approving a valid proposal installs it into the chosen root and records provenance; approving a proposal whose metadata is invalid installs nothing; approving over an existing skill is refused without an explicit override; no path exists by which a proposal installs itself.
- **Move** — moving a skill relocates it and leaves the source gone; `--keep-source` copies it; the destination holding a skill of the same name is refused without an explicit override; the command names the roles that referenced the moved skill; references are rewritten only when the explicit flag is given, and a rewritten reference names the destination tier explicitly; a move into the shared root is reported as an uncommitted working-tree change and a move out of it as an uncommitted deletion.
- **Removal and neutralisation** — removing a skill reports the roles that reference it and refuses without an explicit override; a disabled skill is not offered but remains in the store and the ledger; a refusal recorded in one ledger does not appear in the other.
- **Isolation** — trail cleanup, worktree cleanup, and cache purging leave both roots untouched; the store is never written by a Bee run.
- **Script adapter** — a script Bee receives the skill manifest in its environment, with absolute paths, and a script that ignores it is unaffected.
- **Command surface** — the group invoked bare prints help and changes nothing; the listing reports the tier, the state, and the roles for each skill, and marks a shadowed definition; a dry run of any mutating command leaves the store byte-identical and prints the plan; an ambiguous approval without a trail names the candidates instead of choosing; the read commands emit a parseable machine contract on request and the mutating commands do not depend on one; a reference that the runtime would not resolve is refused by the command that would act on it.

Prior art in-repo: the colony configuration load and validation tests, the run-directory and run-record tests, the launch resolution tests for the merge and environment layers, the adapter capability tests for the session-log resolver, the runtime dispatch tests over rendered prompts, the artifact comb tests including the path-escape cases, the export include tests, the completion-contract tests, the command-level tests for the cue and task listings and their table output, and the dry-run and force-flag tests on the nuc import.

## Out of Scope

- Writing skills into client-specific discovery directories, or configuring a client to point at a skills directory. This is the native bridge, a later phase of this spec.
- Remote skill catalogs, HTTP-sourced skills, or any skill distribution mechanism beyond the two local roots.
- Automatic approval of a proposal, under any policy, flag, or colony setting.
- A skill marketplace, a skill registry across colonies, or publishing a skill to a team catalog.
- A graphical skill editor, or any console surface that writes a skill's body.
- Editing a skill's body in place from the console. The store is plain files and the Beekeeper's editor is the editor.
- Enforcing that a Bee followed a skill. The layer records what was offered and what was reached for; it does not police compliance.
- Accounting for the token cost of a skill separately from the run's usage, or per-skill cost reporting.
- Semantic versioning of skills, or a dependency or compatibility model between skills.
- Skills that reference other skills, or any transitive resolution.
- Per-task or per-sector skill selection. Selection is per role.
- Skill selection by the task's intent, by the task's text, or by any automatic matching. The gate is declared, not inferred.
- Changing what a role's prompt templates contain, or introducing prompt composition that includes a skill body.
- Changing how the existing note-producing intent of the Archivist works.
- Carrying skills in a portable Bee pack. The pack format is unchanged by this spec.
- Reading a client's own skill configuration to discover what the client would have offered on its own.
- Fetching a skill from a URL, from another colony, or from a catalog. The store is filled by hand, by a move, or by approval, and nothing is downloaded.
- An interactive full-screen browser for the store. The listing is a table and the store is a directory; a browsing UI is the console's job, in a later phase.
- A command that writes a commit. A store change that must be shared is a change the Beekeeper commits.
- Renaming a skill. A rename is a new skill and a removed one, because a name is an identity that other files and roles refer to.

## Further Notes

- **Phasing.** Phase 1 is the store, its ledger, selection, the advertisement, the run record, and the reading half of the command group — `list`, `show`, `validate` — plus `move`, `remove`, `disable`, and `enable`, which need nothing but the store itself. It is the implementable core and needs no new event kind. Phase 2 is usage attribution and the export section. Phase 3 is the proposal and the decisions on it — `proposals`, `approve`, `reject` — with the console surface; the refusals and approvals it records go into the ledger Phase 1 already introduced. Phase 4 is the native client bridge. The phases are ordered so that each is useful alone: a colony with Phase 1 can give a Bee a procedure, know it was offered, and manage the store from the shell; Phases 2 and 3 add the measurement and the origination; Phase 4 improves the ergonomics for clients that have them.
- **The advertisement block is the compatibility surface.** Clients advertise skills to a model as a name-and-description block, and Paseka now produces the same shape. A Bee reading a Paseo-advertised skill is reading a file the client would also have read, at a path the client would also have accepted, so the two mechanisms do not conflict when the native bridge lands — a skill advertised twice is idempotent from the Bee's point of view, and a skill offered natively is one the runtime also counted.
- **Provenance in the skill's own metadata.** The standard's metadata field is a string-to-string map, which is exactly enough for a trail identifier, a role name, an approval timestamp, and a content hash. Keeping provenance inside the skill rather than only in the ledger means it travels with the file — including into another client's directory, and into another colony — which is the property that makes an approved skill explainable rather than mysterious.
- **The native bridge's open question.** Every client that supports skills reads a directory of them, and OpenCode accepts an explicit list of directories in its configuration, so the bridge is technically small. What is not settled is where a bridged directory lives for a role that works in the repository root rather than in a worktree: a directory of links placed there would appear in the Beekeeper's working tree, and a colony that cannot answer "where do the links go, and who removes them" should not ship the bridge. In a worktree the answer is already obvious, and the bridge can start there.
- **Attribution is a measurement, not a proof.** A use recorded from a transcript is evidence that a skill's file was read, not evidence that its instructions were followed. The export therefore reports what was reached for, and the wording of that report should not drift toward claiming compliance.
- **The store is deliberately not a client directory.** It is tempting to make the repository's shared root the same directory the clients already read, so that no delivery work is needed at all. That would trade away the per-role gate, the run record, and the attribution, all of which are the reason this spec exists; a client that advertises every skill it finds to every agent it starts cannot express "this procedure belongs to the release role only".
- **A skill is content that a Bee wrote about how to work.** The whole layer is only safe because of the separation in decision 16: the store holds procedures, the role configuration decides who uses them, and the Beekeeper approves both halves separately. Any future feature that collapses those two — a proposal that also edits a role's list, an approval that also activates — should be read as an escalation and rejected.
- **The command group is the layer's only human surface in the shell.** Ten verbs, one namespace, one reference syntax. The design pressure that shaped it is that six of the ten change a reviewed artifact, so every mutating verb previews its plan, refuses on a conflict, and names its repository consequence. A store command that reported success without saying "the working tree now has one untracked file" would leave the Beekeeper believing the team has a procedure the team does not have.
- **The listing's roles column is the reason the listing exists.** A store is easy to list; what a Beekeeper cannot answer by looking is which roles actually carry a procedure, and therefore whether removing or moving one is safe. That answer is computed from the role configurations rather than remembered, and it is the first thing the table shows after the name.
- **Refusal is a recorded state, not an absence.** Both a refused proposal and a disabled skill are recorded so that the same thing is not offered twice and the trail's history shows it was considered. This matters for proposals in particular: a proposal that was looked at and turned down is a decision, and a proposal nobody looked at is not the same thing.
