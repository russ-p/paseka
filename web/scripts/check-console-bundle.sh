#!/bin/sh
# Guard for the Queen Console production bundle (spec 051).
#
# The console is served at `/`, so the SvelteKit base path is empty and every
# route and `_app` asset is root-relative. A base-path regression compiles and
# tests fine and still ships links under the retired `/next` preview prefix,
# which is the one failure the cutover can cause at every link. This inspects the
# emitted entry document and the asset references it carries, and fails the
# build — a test that runs after the bundle shipped is too late.
#
# One file serves every path that builds the console:
#   - `web/package.json` `build`, so release archives (GoReleaser `before.hooks`),
#     the container image (`web-builder` stage), and CI inherit it from the
#     frontend build step they already run;
#   - `scripts/checks.sh`, which executes the trusted colony copy against the
#     tree under review, so a diff cannot neuter the check that judges it;
#   - by hand: `sh web/scripts/check-console-bundle.sh [output-dir]`.
#
# The fixtures beside this script prove the guard still rejects a
# preview-prefixed entry document on every run: a guard that cannot fail is not
# a guard.
set -u

script_dir=$(cd "$(dirname "$0")" && pwd)
repo_root=$(cd "$script_dir/../.." && pwd)

usage() {
	cat <<'EOF'
usage: sh web/scripts/check-console-bundle.sh [output-dir]

Checks the built Queen Console bundle for the retired `/next` prefix and for
`_app` assets emitted at the output root. Defaults to
internal/console/ui/dist, where `pnpm --dir web build` writes the bundle.
EOF
}

say_check() { printf 'console bundle check: %s\n' "$*" >&2; }

# bundle_check <dir> reports every problem it finds and returns 1 if any was
# found. It is called for the real bundle and, from a subshell, for the
# fixtures, so it never exits on the first failure.
bundle_check() {
	dir=$1
	fail=0

	if [ ! -d "$dir" ]; then
		say_check "no bundle at $dir — the frontend build did not run"
		return 1
	fi

	entry=
	for candidate in 200.html index.html; do
		if [ -f "$dir/$candidate" ]; then
			entry=$dir/$candidate
			break
		fi
	done
	if [ -z "$entry" ]; then
		say_check "no entry document (200.html or index.html) in $dir"
		return 1
	fi

	# The whole entry document, not just the references: a leftover base buried
	# in markup or an inline module hint is the same regression.
	if hits=$(grep -n -F '/next' "$entry"); then
		say_check "$entry still names the retired preview prefix:"
		printf '%s\n' "$hits" >&2
		say_check "the base path must be empty so routes and _app assets are root-relative (web/vite.config.ts)"
		fail=1
	fi

	refs=$(grep -o -E '(href|src)="[^"]*"' "$entry" | sed -E 's/^(href|src)="//; s/"$//')
	app_refs=0
	for ref in $refs; do
		case $ref in
		/_app/*) app_refs=$((app_refs + 1)) ;;
		esac
	done

	# `_app` is the bundle: it is served from the root, so it must be emitted
	# there rather than under a prefix or beside it.
	app_dir=1
	if [ ! -d "$dir/_app" ]; then
		say_check "no _app directory at the output root: $dir/_app"
		app_dir=0
		fail=1
	elif [ -z "$(find "$dir/_app" -type f -print 2>/dev/null)" ]; then
		say_check "_app is emitted at the output root but holds no assets: $dir/_app"
		app_dir=0
		fail=1
	elif [ "$app_refs" -eq 0 ] && [ "$fail" -eq 0 ]; then
		say_check "the entry document loads no /_app asset, so it is not the built console: $entry"
		fail=1
	fi

	# Every root-relative asset the entry document names has to exist at the
	# output root: a reference that resolves under some other mount is a
	# base-path regression that does not spell `/next`. With no `_app` at the
	# output root the one message above already says why each one is missing.
	[ "$app_dir" -eq 1 ] || return "$fail"
	for ref in $refs; do
		case $ref in
		/_app/*)
			if [ ! -f "$dir/${ref#/}" ]; then
				say_check "entry references $ref but no such asset is emitted at the output root ($dir/${ref#/})"
				fail=1
			fi
			;;
		esac
	done

	return "$fail"
}

case ${1:-} in
-h | --help)
	usage
	exit 0
	;;
esac

out=${1:-$repo_root/internal/console/ui/dist}

bundle_check "$out" || exit 1
say_check "clean ($out)"

# The fixtures are the failure this guard exists for. The preview-prefixed entry
# must be rejected for naming `/next`; the root entry proves the guard is not
# simply rejecting everything.
fixtures=$script_dir/testdata

if output=$(bundle_check "$fixtures/preview-entry" 2>&1); then
	say_check "the preview-prefixed fixture was accepted:"
	printf '%s\n' "$output" >&2
	exit 1
fi
case $output in
*'/next'*) ;;
*)
	say_check "the preview-prefixed fixture failed for the wrong reason:"
	printf '%s\n' "$output" >&2
	exit 1
	;;
esac

if ! output=$(bundle_check "$fixtures/root-entry" 2>&1); then
	say_check "the root-entry fixture was rejected:"
	printf '%s\n' "$output" >&2
	exit 1
fi

say_check "fixtures behave (preview-prefixed rejected, root accepted)"
