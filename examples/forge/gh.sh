#!/usr/bin/env bash
# GitHub (gh) reference driver for Paseka pull-request delivery.
# Home config: forge.command: ["/absolute/path/to/examples/forge/gh.sh"]
# Tokens stay in gh's own login; Paseka never passes them on argv.
set -euo pipefail
exec python3 -c '
import json, subprocess, sys

op = sys.argv[1]
req = json.load(sys.stdin)

def out(obj):
    json.dump(obj, sys.stdout)
    sys.stdout.write("\n")

if op == "capabilities":
    out({"protocolVersion": 1, "ops": ["upsert", "get"]})
    raise SystemExit(0)

head = (req.get("head") or "").strip()
if not head:
    sys.stderr.write("head is required\n")
    raise SystemExit(1)

base = (req.get("base") or "main").strip()

def gh(*args):
    return subprocess.run(["gh", *args], check=False, capture_output=True, text=True)

def list_one():
    r = gh("pr", "list", "--head", head, "--state", "all",
           "--json", "number,url,state,isDraft,headRefName,baseRefName")
    if r.returncode != 0:
        sys.stderr.write(r.stderr or r.stdout or "gh pr list failed\n")
        raise SystemExit(r.returncode or 1)
    rows = json.loads(r.stdout or "[]")
    if len(rows) > 1:
        sys.stderr.write("multiple pull requests for head %s\n" % head)
        raise SystemExit(1)
    return rows[0] if rows else None

def emit(p):
    if not p:
        out({"protocolVersion": 1, "found": False})
        return
    state = str(p.get("state") or "open").lower()
    if state == "merged":
        state = "merged"
    elif state == "closed":
        state = "closed"
    else:
        state = "open"
    out({
        "protocolVersion": 1,
        "found": True,
        "number": int(p.get("number") or 0),
        "url": p.get("url") or "",
        "head": p.get("headRefName") or head,
        "base": p.get("baseRefName") or base,
        "state": state,
        "draft": bool(p.get("isDraft") or False),
    })

if op == "get":
    emit(list_one())
    raise SystemExit(0)

if op != "upsert":
    sys.stderr.write("unsupported op: %s\n" % op)
    raise SystemExit(1)

existing = list_one()
title = req.get("title") or head
body = req.get("body") or ""
draft = bool(req.get("draft"))
if existing is None:
    args = ["pr", "create", "--head", head, "--base", base, "--title", title, "--body", body]
    if draft:
        args.append("--draft")
    r = gh(*args)
    if r.returncode != 0:
        sys.stderr.write(r.stderr or r.stdout or "gh pr create failed\n")
        raise SystemExit(r.returncode or 1)
    emit(list_one())
    raise SystemExit(0)

number = str(existing.get("number") or "")
r = gh("pr", "edit", number, "--title", title, "--body", body)
if r.returncode != 0:
    sys.stderr.write(r.stderr or r.stdout or "gh pr edit failed\n")
    raise SystemExit(r.returncode or 1)
emit(list_one())
' "${1:-}"
