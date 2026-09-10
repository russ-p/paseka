## Standing Trail ticks

When this flight trail is a **Standing Trail** (recurring procedure identity, same `traceId` every tick):

- Procedure memory is the trail comb at `{{.ArtifactsDir}}`, not `{{.Insights}}` (narrative, capped).
- Source of truth: `{{.ArtifactsDir}}/checkpoint.json` (skip lists, issue keys, SHAs). Optional human journal: `{{.ArtifactsDir}}/journal/YYYY-MM-DD.md`.
- **Read the checkpoint first.** Do not re-triage keys already listed. Update the checkpoint when the skip list or recorded state changes.
- Write checkpoint JSON atomically from your point of view (write a temp file in the comb, then replace) so a crash is less likely to leave truncated JSON.
- Runtime announces `artifact.written` only when comb file content actually changed (014 scan-flush). Do not emit `artifact.written` on every save. Downstream bees should honor `artifactKind` `checkpoint`. Dated journal files under `journal/` use the date as `artifactKind` (014 basename-stem); ignore them unless you are writing the journal.
- A standing tick **observes and records**. If product work is needed (feature, hotfix, isolated implementation), **spawn a bloom trail**: `paseka cue run <bloom-cue> "…"` **without** a standing binding (new `traceId`), or `paseka signal` / `event emit` with a **new** `traceId`. Do not `task.plan` a builder on this standing id. Isolated `code.proposal` on a standing trail is a colony smell.
- Ad-hoc `bee run` / `bee chat` on this id shares the same comb and does **not** apply stipend (stipend is cue-tick scoped).
