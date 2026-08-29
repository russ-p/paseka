#!/usr/bin/env bash
# Render Archify JSON specs under docs/ into sibling HTML files.
# HTML is gitignored; docs CI runs this before `mkdocs build`.
#
# Usage:
#   scripts/render-archify.sh              # deliver (write HTML)
#   scripts/render-archify.sh deliver
#   scripts/render-archify.sh validate     # schema + showcase checks, no HTML
#
# Override the pin with ARCHIFY_REF / ARCHIFY_REPO, or point at a local
# checkout with ARCHIFY_BIN=/path/to/archify/bin/archify.mjs
set -euo pipefail

mode="${1:-deliver}"
if [[ "$mode" != "deliver" && "$mode" != "validate" ]]; then
  echo "usage: $0 [deliver|validate]" >&2
  exit 2
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

if ! command -v node >/dev/null 2>&1; then
  echo "render-archify: node is required (Node 18+)" >&2
  exit 1
fi

# Pin matches the Archify 2.16 line used to author the Hive Runtime map.
# Update both values together when bumping.
ARCHIFY_REPO="${ARCHIFY_REPO:-https://github.com/tt-a1i/archify}"
ARCHIFY_REF="${ARCHIFY_REF:-0853a805003514776bef3593ecca091409828902}"

resolve_cli() {
  local candidate="$1"
  [[ -n "$candidate" ]] || return 1
  if [[ -f "$candidate/bin/archify.mjs" ]]; then
    printf '%s\n' "$candidate/bin/archify.mjs"
    return 0
  fi
  if [[ -f "$candidate/archify/bin/archify.mjs" ]]; then
    printf '%s\n' "$candidate/archify/bin/archify.mjs"
    return 0
  fi
  return 1
}

fetch_pinned_cli() {
  local cache="$root/.tmp/archify/$ARCHIFY_REF"
  local cli
  if cli="$(resolve_cli "$cache" 2>/dev/null)"; then
    printf '%s\n' "$cli"
    return 0
  fi

  if ! command -v curl >/dev/null 2>&1; then
    echo "render-archify: curl is required to fetch Archify $ARCHIFY_REF" >&2
    exit 1
  fi

  mkdir -p "$cache"
  local tarball="$root/.tmp/archify/$ARCHIFY_REF.tar.gz"
  local url="$ARCHIFY_REPO/archive/${ARCHIFY_REF}.tar.gz"
  echo "render-archify: fetching $url" >&2
  curl -fsSL "$url" -o "$tarball"
  tar -xzf "$tarball" -C "$cache" --strip-components 1
  rm -f "$tarball"

  if ! cli="$(resolve_cli "$cache")"; then
    echo "render-archify: fetched archive has no bin/archify.mjs" >&2
    exit 1
  fi
  printf '%s\n' "$cli"
}

if [[ -n "${ARCHIFY_BIN:-}" ]]; then
  cli="$ARCHIFY_BIN"
elif [[ -n "${ARCHIFY_HOME:-}" ]] && cli="$(resolve_cli "$ARCHIFY_HOME")"; then
  :
else
  cli="$(fetch_pinned_cli)"
fi

if [[ ! -f "$cli" ]]; then
  echo "render-archify: Archify CLI not found: $cli" >&2
  exit 1
fi

echo "render-archify: using $cli ($mode)"

mapfile -d '' specs < <(find "$root/docs" -type f \( \
  -name '*.architecture.json' -o \
  -name '*.workflow.json' -o \
  -name '*.sequence.json' -o \
  -name '*.dataflow.json' -o \
  -name '*.lifecycle.json' \
\) -print0 | sort -z)

if [[ ${#specs[@]} -eq 0 ]]; then
  echo "render-archify: no Archify specs under docs/" >&2
  exit 1
fi

failed=0
for spec in "${specs[@]}"; do
  [[ -n "$spec" ]] || continue
  meta="$(node -e '
    const fs = require("fs");
    const d = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
    const type = d.diagram_type || "";
    const quality = (d.meta && d.meta.quality_profile) || "showcase";
    if (!type) {
      console.error("missing diagram_type");
      process.exit(1);
    }
    process.stdout.write(type + "\n" + quality + "\n");
  ' "$spec")"
  diagram_type="$(printf '%s\n' "$meta" | sed -n '1p')"
  quality="$(printf '%s\n' "$meta" | sed -n '2p')"
  rel="${spec#"$root/"}"

  args=(node "$cli" "$mode" "$diagram_type" "$spec" --quality "$quality" --json --repo-root "$root")
  if [[ "$mode" == "deliver" ]]; then
    out="${spec%.json}"
    out="${out%.architecture}"
    out="${out%.workflow}"
    out="${out%.sequence}"
    out="${out%.dataflow}"
    out="${out%.lifecycle}"
    out="${out}.html"
    args=(node "$cli" deliver "$diagram_type" "$spec" "$out" --quality "$quality" --json --repo-root "$root")
  fi

  echo "render-archify: $mode $rel"
  if ! "${args[@]}"; then
    echo "render-archify: failed $rel" >&2
    failed=1
  fi
done

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
