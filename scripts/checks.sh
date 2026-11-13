#!/bin/sh
# Mechanical gate for AFK builder runs (spec 046).
#
# The runtime executes this file from the colony checkout ($COLONY_ROOT) while
# the working directory is the tree under review, so a diff cannot weaken the
# gate that judges it. Run it by hand to check any tree — the working
# directory decides what is checked:
#
#   sh scripts/checks.sh
#
# Covers Go formatting, lint, module build, and module tests. It adds the Queen
# Console UI checks when the diff touches web/, and also when the changed paths
# cannot be determined: an unscoped gate runs more, never less. The UI half ends
# with the console bundle guard (web/scripts/check-console-bundle.sh), the same
# file `pnpm --dir web build` runs for releases and the container image.
set -u

say() { printf 'checks: %s\n' "$*"; }

workspace=$(pwd)
say "workspace $workspace"
if [ -n "${PASEKA_COLONY_ROOT:-}" ]; then
	say "colony ${PASEKA_COLONY_ROOT}"
	head=$(git -C "$PASEKA_COLONY_ROOT" rev-parse --short HEAD 2>/dev/null) && say "colony head $head"
	digest=$(sha256sum "$0" 2>/dev/null | cut -d' ' -f1) && say "gate script $digest"
fi

# --- Go ---------------------------------------------------------------------

command -v gofmt >/dev/null 2>&1 || { say "gofmt is not installed"; exit 1; }
command -v golangci-lint >/dev/null 2>&1 || { say "golangci-lint is not installed"; exit 1; }
git ls-files -co --exclude-standard '*.go' >/dev/null 2>&1 \
	|| { say "cannot list Go files (git ls-files failed)"; exit 1; }

# Scope formatting to the repository's Go files: a full-tree walk would enter
# gitignored caches (.tmp/, .paseka/worktrees/) and third-party code.
unformatted=$(git ls-files -z -co --exclude-standard '*.go' | xargs -0 -r gofmt -l) \
	|| { say "gofmt failed"; exit 1; }
if [ -n "$unformatted" ]; then
	say "gofmt needs formatting:"
	printf '%s\n' "$unformatted"
	exit 1
fi
say "gofmt clean"

# The lint configuration comes from the trusted tree, not from the diff: a
# weakened .golangci.yml would otherwise pass the gate.
lint_config=${PASEKA_COLONY_ROOT:-$workspace}/.golangci.yml
if [ -f "$lint_config" ]; then
	golangci-lint run --timeout 10m --config "$lint_config" || exit 1
else
	golangci-lint run --timeout 10m || exit 1
fi
say "golangci-lint clean"

go build ./... || exit 1
say "go build clean"

go test ./... || exit 1
say "go test clean"

# --- Queen Console UI -------------------------------------------------------

ui=0
if changes=$(git status --porcelain -uall 2>/dev/null); then
	# git quotes paths that need it, so a quoted "web/... is still a match.
	if printf '%s\n' "$changes" | grep -qE '(^|[[:space:]"])web/'; then
		ui=1
	fi
else
	say "git status failed — checking the UI regardless"
	ui=1
fi

if [ "$ui" -eq 0 ]; then
	say "no web/ changes — console checks skipped"
	exit 0
fi

command -v pnpm >/dev/null 2>&1 \
	|| { say "pnpm is not installed but the change touches web/"; exit 1; }
if [ ! -d web/node_modules ]; then
	# A trace worktree is created by git worktree add, so installed
	# dependencies are absent; --frozen-lockfile keeps the gate from rewriting
	# the lockfile in the tree under review.
	say "installing web dependencies (frozen lockfile, prefer offline store)"
	pnpm --dir web install --frozen-lockfile --prefer-offline \
		|| { say "web dependency install failed"; exit 1; }
fi

pnpm --dir web exec svelte-kit sync || exit 1
# Invoke the binaries rather than the package.json scripts: package.json is part
# of the diff under review, so a neutered `check` script would pass this gate.
pnpm --dir web exec svelte-check --tsconfig ./tsconfig.json || exit 1
say "svelte-check clean"

pnpm --dir web exec vitest run || exit 1
say "vitest clean"

pnpm --dir web exec vite build || exit 1
say "vite build clean"

# The console is served at the root, so a bundle still carrying the retired
# /next base ships broken links from every build path. The guard runs here as
# well as inside `pnpm --dir web build` (release archives, the container image,
# CI) because this gate invokes vite directly. It comes from the trusted colony
# copy so a diff cannot weaken the check that judges it.
guard=${PASEKA_COLONY_ROOT:-$workspace}/web/scripts/check-console-bundle.sh
if [ ! -f "$guard" ]; then
	say "console bundle guard missing: $guard"
	exit 1
fi
sh "$guard" "$workspace/internal/console/ui/dist" || exit 1
say "console bundle guard clean"
