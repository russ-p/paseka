#!/usr/bin/env python3
"""Print the architecture-limit report as one JSON object, cheapest target first.

Read-only. It runs the same warn-only lint pass as scripts/checks.sh and
normalizes the findings, so a standing refactor tick can pick exactly one target
without parsing linter messages itself.

    python3 scripts/refactor-targets.py [--root PATH] [--max-size N]

Output shape:

    {"root": "...", "generatedAt": "...", "counts": {"cyclop": 335, ...},
     "total": 757,
     "candidates": [{"file": "cmd/paseka/bee.go", "line": 25,
                     "symbol": "newBeeRunCmd", "cost": 1, "size": 17,
                     "findings": [{"linter": "cyclop", "value": 11, "limit": 10}]}],
     "excluded": {"tooLarge": 0, "unparsed": 0}}

`cost` is how far a finding sits above its limit (for maintidx, how far the
index sits below 20), so the first candidate is the cheapest warning to remove.
A function reported by several linters is one candidate: fixing it should clear
all of them. `--max-size` drops targets whose worst metric is already far past
the limit; those are rewrites, not one-commit tidy-ups.

The linter list mirrors the architecture limits in .golangci.yml: update both
when a linter joins or leaves that block.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path

ARCH_LINTERS = ("cyclop", "gocognit", "funlen", "maintidx", "interfacebloat")

# Per-linter message shapes, captured from the linters themselves. `value` is the
# measured number, `limit` the threshold from .golangci.yml.
PATTERNS = {
    "cyclop": re.compile(
        r"calculated cyclomatic complexity for function (?P<symbol>\S+) is "
        r"(?P<value>\d+), max is (?P<limit>\d+)"
    ),
    "gocognit": re.compile(
        r"cognitive complexity (?P<value>\d+) of func `(?P<symbol>[^`]+)` "
        r"is high \(> (?P<limit>\d+)\)"
    ),
    # Two shapes: too many lines, and too many statements.
    "funlen": re.compile(
        r"Function '(?P<symbol>[^']+)' (?:is too long|has too many statements) "
        r"\((?P<value>\d+) > (?P<limit>\d+)"
    ),
    "maintidx": re.compile(
        r"Function name: (?P<symbol>[^,]+), Cyclomatic Complexity: "
        r"(?P<cyclo>\d+), .*Maintainability Index: (?P<value>\d+)"
    ),
    "interfacebloat": re.compile(
        r"the interface has more than (?P<limit>\d+) methods: (?P<value>\d+)"
    ),
}

MAINTIDX_LIMIT = 20


def fail(message: str) -> "NoReturn":
    print(f"refactor-targets: {message}", file=sys.stderr)
    raise SystemExit(1)


def colony_root(explicit: str | None) -> Path:
    if explicit:
        return Path(explicit).resolve()
    try:
        out = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            capture_output=True,
            text=True,
            check=True,
        )
    except (OSError, subprocess.CalledProcessError):
        return Path.cwd().resolve()
    return Path(out.stdout.strip()).resolve()


def dirty_paths(root: Path) -> set[str]:
    """Files with uncommitted changes, relative to the colony root.

    A refactor bloom is cut from HEAD, so a finding inside a file that is already
    dirty describes code the Builder would not see. Those targets are skipped
    rather than dispatched stale.
    """
    try:
        proc = subprocess.run(
            ["git", "-C", str(root), "status", "--porcelain", "-uall", "-z"],
            capture_output=True,
            text=True,
            check=True,
        )
    except (OSError, subprocess.CalledProcessError):
        return set()
    paths = set()
    for entry in proc.stdout.split("\0"):
        # "XY <path>"; rename entries carry the new path first, the old one next.
        if len(entry) > 3:
            paths.add(entry[3:])
    return paths


def run_lint(root: Path) -> dict:
    config = root / ".golangci.yml"
    if not config.is_file():
        fail(f"no lint configuration at {config}")
    with tempfile.TemporaryDirectory(prefix="paseka-refactor-") as tmp:
        report = Path(tmp) / "advisory.json"
        cmd = [
            "golangci-lint",
            "run",
            "--timeout",
            "10m",
            "--config",
            str(config),
            "--enable-only",
            ",".join(ARCH_LINTERS),
            "--uniq-by-line=false",
            "--issues-exit-code",
            "0",
            "--max-issues-per-linter",
            "0",
            "--max-same-issues",
            "0",
            "--show-stats=false",
            "--output.text.path",
            os.devnull,
            "--output.json.path",
            str(report),
        ]
        try:
            proc = subprocess.run(cmd, cwd=root, capture_output=True, text=True)
        except OSError as err:
            fail(f"cannot run golangci-lint: {err}")
        if proc.returncode != 0:
            fail(
                "golangci-lint failed "
                f"(exit {proc.returncode}): {(proc.stderr or proc.stdout).strip()}"
            )
        if not report.is_file():
            fail("golangci-lint wrote no JSON report")
        return json.loads(report.read_text())


def interface_symbol(root: Path, path: str, line: int) -> str:
    """Name the interface a bloat finding points at, from its declaration line."""
    try:
        source = (root / path).read_text(errors="replace").splitlines()
    except OSError:
        return f"interface at {path}:{line}"
    if 0 < line <= len(source):
        match = re.search(r"\btype\s+([A-Za-z_]\w*)\s+interface\b", source[line - 1])
        if match:
            return match.group(1)
    return f"interface at {path}:{line}"


def normalize(root: Path, issue: dict) -> dict | None:
    linter = issue.get("FromLinter", "")
    pattern = PATTERNS.get(linter)
    if pattern is None:
        return None
    text = issue.get("Text", "")
    match = pattern.search(text)
    if not match:
        return None
    groups = match.groupdict()
    value = int(groups["value"])
    limit = MAINTIDX_LIMIT if linter == "maintidx" else int(groups["limit"])
    path = issue["Pos"]["Filename"]
    # golangci-lint may emit paths relative to its own workdir with ../ hops.
    path = os.path.relpath(os.path.normpath(str(root / path)), root)
    line = int(issue["Pos"]["Line"])
    telemetry = {"linter": linter, "value": value, "limit": limit}

    if linter == "maintidx":
        cost = max(limit - value, 0)
        size = int(groups.get("cyclo", value))
    else:
        cost = max(value - limit, 0)
        size = value
    if linter == "interfacebloat":
        name = interface_symbol(root, path, line)
    else:
        name = groups["symbol"].strip()

    return {
        "linter": linter,
        "file": path,
        "line": line,
        "symbol": name,
        "value": value,
        "limit": limit,
        "cost": cost,
        "size": size,
        "text": text,
    }


def build_report(root: Path, raw: dict, max_size: int, include_dirty: bool) -> dict:
    counts: dict[str, int] = {}
    grouped: dict[tuple[str, str], dict] = {}
    unparsed = 0

    for issue in raw.get("Issues") or []:
        linter = issue.get("FromLinter", "")
        counts[linter] = counts.get(linter, 0) + 1
        row = normalize(root, issue)
        if row is None:
            unparsed += 1
            continue
        key = (row["file"], row["symbol"])
        target = grouped.get(key)
        if target is None:
            grouped[key] = {
                "file": row["file"],
                "line": row["line"],
                "symbol": row["symbol"],
                "cost": row["cost"],
                "size": row["size"],
                "findings": [
                    {"linter": row["linter"], "value": row["value"], "limit": row["limit"]}
                ],
            }
            continue
        target["line"] = min(target["line"], row["line"])
        target["cost"] = min(target["cost"], row["cost"])
        target["size"] = max(target["size"], row["size"])
        target["findings"].append(
            {"linter": row["linter"], "value": row["value"], "limit": row["limit"]}
        )

    candidates: list[dict] = []
    too_large = 0
    dirty = dirty_paths(root) if not include_dirty else set()
    dirty_skipped = 0
    for target in grouped.values():
        if target["file"] in dirty:
            dirty_skipped += 1
            continue
        if target["size"] > max_size:
            too_large += 1
            continue
        target["findings"].sort(key=lambda item: item["linter"])
        candidates.append(target)

    candidates.sort(key=lambda item: (item["cost"], item["size"], item["file"], item["line"]))

    return {
        "root": str(root),
        "generatedAt": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "counts": dict(sorted(counts.items())),
        "total": sum(counts.values()),
        "maxSize": max_size,
        "candidates": candidates,
        "excluded": {
            "tooLarge": too_large,
            "dirty": dirty_skipped,
            "unparsed": unparsed,
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--root", help="colony root (default: git top level of cwd)")
    parser.add_argument(
        "--max-size",
        type=int,
        default=25,
        help="drop targets whose worst metric exceeds this (default: 25)",
    )
    parser.add_argument(
        "--include-dirty",
        action="store_true",
        help="keep targets in files with uncommitted changes",
    )
    args = parser.parse_args()

    root = colony_root(args.root)
    raw = run_lint(root)
    report = build_report(root, raw, args.max_size, args.include_dirty)
    json.dump(report, sys.stdout, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
