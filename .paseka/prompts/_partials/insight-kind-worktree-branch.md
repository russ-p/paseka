### `worktree.branch` — worktree branch name

Emit when planning isolated implementation (`decision=plan` or `decision=triage`). Optional on grill / clarify / reject. Last-write-wins for the trace git branch; path stays `.paseka/worktrees/<traceId>/`.

Naming guidance (not protocol): lowercase kebab slug from title — `feature/<slug>` for plan, `hotfix/<slug>` (or `fix/<slug>`) for triage.

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"worktree.branch","branch":"feature/live-bees-header"}}
EOF
```
