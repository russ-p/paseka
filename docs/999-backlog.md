# Backlog

## Deferred Ideas

### Replace `result.txt` with a clearer log artifact

Context:
- The current AFK adapter flow still uses `result.txt` as a familiar run artifact inherited from the earliest bash-based prototype.
- During the `run.summary` migration, the runtime should stop depending on this file for success semantics.
- The file may still be useful as a human-readable log written by runtime after summary normalization.

Backlog item:
- Consider renaming `result.txt` to `summary.md` or another clearer log-oriented filename after the runtime-first `INSIGHT/run.summary` migration is complete.

Why deferred:
- Renaming the file now would broaden the migration unnecessarily.
- It would require coordinated updates across prompt templates, runtime context, docs, tests, and any external assumptions about run-directory layout.
- Keeping `result.txt` temporarily as a compatibility log path reduces migration risk while the success model is being changed.

Exit criteria for revisiting:
- AFK run success no longer depends on reading `result.txt`.
- Runtime auto-publishes or enforces `INSIGHT/run.summary` consistently.
- Prompt templates and docs no longer describe the file as the run-success handshake.
