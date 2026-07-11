#!/usr/bin/env bash
# Generate docs/llms-full.txt from the agent-config guide set.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
docs="$root/docs"
out="$docs/llms-full.txt"

files=(
  010-bee-config.md
  004-prompt-templates.md
  008-bee-routing.md
  009-insight-kinds.md
  003-architecture.md
)

{
  cat <<'EOF'
# Paseka — agent configuration corpus

> Single-fetch Markdown for configuring bees (YAML, prompts, event emit) without reading the Go codebase.
> Index: https://russ-p.github.io/paseka/llms.txt

Colony paths: `.paseka/bees/`, `.paseka/prompts/`, `.paseka/runs/`, `.paseka/worktrees/`.
Machine-local: `~/.config/paseka/<project-slug>/` (secrets only — no prompts).
Bus contracts: SIGNAL, INSIGHT, MUTATION, VERIFICATION.
EOF

  for f in "${files[@]}"; do
    path="$docs/$f"
    if [[ ! -f "$path" ]]; then
      echo "missing source: $path" >&2
      exit 1
    fi
    printf '\n\n---\n# Source: %s\n---\n\n' "$f"
    cat "$path"
  done
} >"$out"

echo "wrote $out ($(wc -c <"$out") bytes)"
