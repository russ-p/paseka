#!/bin/sh
# emit.sh — publish one Paseka event and prove it landed.
#
#   printf '%s' '{"kind":"context.note","summary":"…"}' \
#     | sh emit.sh --type INSIGHT --defer
#   sh emit.sh --type VERIFICATION --payload '{"kind":"task.completed",…}'
#
# --trace, --agent, and --root carry the run header values: the Flight trail, the
# agent run id, and the Colony line. They fall back to PASEKA_TRACE_ID,
# PASEKA_AGENT_ID, and PASEKA_COLONY_ROOT, which only the script adapter exports.
# --validate-only checks the envelope and publishes nothing.
set -eu

usage() {
  cat <<'USAGE'
usage: emit.sh --type TYPE [--payload JSON] [--defer] [--validate-only]
               [--trace ID] [--agent ID] [--root COLONY_ROOT]

  TYPE           SIGNAL | INSIGHT | MUTATION | VERIFICATION
  --payload      payload object; read from stdin when omitted
  --defer        queue for flush on successful run exit (live by default)
  --validate-only  validate and stop; nothing is published or queued
USAGE
}

BIN=${PASEKA_BIN:-paseka}
TYPE=
PAYLOAD=
TRACE=${PASEKA_TRACE_ID:-}
AGENT=${PASEKA_AGENT_ID:-}
ROOT=${PASEKA_COLONY_ROOT:-}
DEFER=0
VALIDATE_ONLY=0

while [ $# -gt 0 ]; do
  case "$1" in
    --type) TYPE=${2:-}; shift 2 ;;
    --payload) PAYLOAD=${2:-}; shift 2 ;;
    --trace) TRACE=${2:-}; shift 2 ;;
    --agent) AGENT=${2:-}; shift 2 ;;
    --root) ROOT=${2:-}; shift 2 ;;
    --defer) DEFER=1; shift ;;
    --validate-only) VALIDATE_ONLY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "emit.sh: unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

case "$TYPE" in
  SIGNAL|INSIGHT|MUTATION|VERIFICATION) ;;
  *) echo "emit.sh: --type must be SIGNAL, INSIGHT, MUTATION, or VERIFICATION" >&2; exit 2 ;;
esac

command -v "$BIN" >/dev/null 2>&1 || { echo "emit.sh: $BIN not found; set PASEKA_BIN" >&2; exit 2; }
command -v jq >/dev/null 2>&1 || { echo "emit.sh: jq is required" >&2; exit 2; }

if [ -z "$PAYLOAD" ]; then
  PAYLOAD=$(cat)
fi
[ -n "$PAYLOAD" ] || { echo "emit.sh: no payload (--payload or stdin)" >&2; exit 2; }

KIND=$(printf '%s' "$PAYLOAD" | jq -r '.kind // empty' 2>/dev/null || true)
[ -n "$KIND" ] || { echo "emit.sh: payload.kind is required" >&2; exit 2; }

[ -n "$TRACE" ] || { echo "emit.sh: no trace id (--trace or PASEKA_TRACE_ID)" >&2; exit 2; }
[ -n "$AGENT" ] || { echo "emit.sh: no agent id (--agent or PASEKA_AGENT_ID)" >&2; exit 2; }

paseka_cli() {
  if [ -n "$ROOT" ]; then
    "$BIN" -C "$ROOT" "$@"
  else
    "$BIN" "$@"
  fi
}

ENVELOPE=$(jq -cn --arg t "$TRACE" --arg a "$AGENT" --arg ty "$TYPE" --argjson p "$PAYLOAD" \
  '{traceId:$t,agentId:$a,type:$ty,payload:$p}') || {
  echo "emit.sh: payload is not valid JSON" >&2
  exit 2
}

# Validate first: no side effects, and the failure names the field to fix.
if ! printf '%s' "$ENVELOPE" | paseka_cli event validate --stdin; then
  echo "emit.sh: validation failed — fix details[].path above; nothing was published" >&2
  exit 1
fi
if [ "$VALIDATE_ONLY" -eq 1 ]; then
  exit 0
fi

if [ "$DEFER" -eq 1 ]; then
  if ! printf '%s' "$ENVELOPE" | paseka_cli event emit --defer --stdin; then
    echo "emit.sh: deferred emit failed — see references/troubleshooting.md" >&2
    exit 1
  fi
else
  if ! printf '%s' "$ENVELOPE" | paseka_cli event emit --stdin; then
    echo "emit.sh: live emit failed — see references/troubleshooting.md" >&2
    exit 1
  fi
fi

printf '%s\n' "--- proof"
if [ "$DEFER" -eq 1 ]; then
  paseka_cli event pending --trace "$TRACE" --agent "$AGENT"
  exit 0
fi

if [ -n "$ROOT" ]; then
  base=$ROOT
else
  base=$(pwd)
fi
LOG="$base/.paseka/runs/$TRACE/$AGENT/events.ndjson"
if [ -f "$LOG" ]; then
  printf 'recorded %s at %s\n' "$KIND" \
    "$(jq -r --arg k "$KIND" 'select(.payload.kind==$k)|.createdAt' "$LOG" | tail -1)"
else
  printf 'no run log at %s\n' "$LOG"
  printf 'the event is in the trail record: %s/.paseka/runs/%s/trail-events.ndjson\n' "$base" "$TRACE"
fi
