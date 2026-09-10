#!/usr/bin/env bash
# Gitea (tea) reference driver for Paseka pull-request delivery.
# Home config: forge.command: ["/absolute/path/to/examples/forge/tea.sh"]
# Tokens stay in tea's own login; Paseka never passes them on argv.
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

def tea(*args):
    return subprocess.run(["tea", *args], check=False, capture_output=True, text=True)

def fetch_list(state=None):
    extra = ["--state", state] if state else []
    r = tea("pulls", "list", "--output", "json", *extra)
    if r.returncode != 0:
        r = tea("pr", "list", "--output", "json", *extra)
    if r.returncode != 0:
        return None, r
    data = json.loads(r.stdout or "[]")
    rows = data if isinstance(data, list) else data.get("data") or data.get("pulls") or []
    return rows, r

def list_pulls(include_closed=False):
    rows = []
    if include_closed:
        fetched, r = fetch_list("all")
        if fetched is None:
            open_rows, r_open = fetch_list()
            closed_rows, r_closed = fetch_list("closed")
            if open_rows is None and closed_rows is None:
                err = r or r_open or r_closed
                sys.stderr.write((err.stderr if err else "") or (err.stdout if err else "") or "tea list failed\n")
                raise SystemExit((err.returncode if err else 1) or 1)
            rows = (open_rows or []) + (closed_rows or [])
        else:
            rows = fetched
    else:
        fetched, r = fetch_list()
        if fetched is None:
            sys.stderr.write(r.stderr or r.stdout or "tea list failed\n")
            raise SystemExit(r.returncode or 1)
        rows = fetched
    hits = []
    seen = set()
    for p in rows:
        h = p.get("head") or {}
        name = h.get("ref") if isinstance(h, dict) else h
        name = name or p.get("head") or ""
        if not (name == head or str(name).endswith("/" + head)):
            continue
        key = p.get("number") or p.get("html_url") or p.get("url") or name
        if key in seen:
            continue
        seen.add(key)
        hits.append(p)
    if len(hits) > 1:
        sys.stderr.write("multiple pull requests for head %s\n" % head)
        raise SystemExit(1)
    return hits[0] if hits else None

def emit(p):
    if not p:
        out({"protocolVersion": 1, "found": False})
        return
    url = p.get("html_url") or p.get("url") or ""
    state = str(p.get("state") or "open").lower()
    if p.get("merged") or p.get("merged_at") or state == "merged":
        state = "merged"
    elif state.startswith("close"):
        state = "closed"
    else:
        state = "open"
    h = p.get("head") or {}
    b = p.get("base") or {}
    out({
        "protocolVersion": 1,
        "found": True,
        "number": int(p.get("number") or 0),
        "url": url,
        "head": (h.get("ref") if isinstance(h, dict) else h) or head,
        "base": (b.get("ref") if isinstance(b, dict) else b) or base,
        "state": state,
        "draft": bool(p.get("draft") or False),
    })

if op == "get":
    emit(list_pulls(include_closed=True))
    raise SystemExit(0)

if op != "upsert":
    sys.stderr.write("unsupported op: %s\n" % op)
    raise SystemExit(1)

existing = list_pulls()
title = req.get("title") or head
body = req.get("body") or ""
draft = bool(req.get("draft"))
if existing is None:
    args = ["pulls", "create", "--head", head, "--base", base, "--title", title]
    if body:
        args += ["--description", body]
    if draft:
        args += ["--draft"]
    r = tea(*args)
    if r.returncode != 0:
        sys.stderr.write(r.stderr or r.stdout or "tea create failed\n")
        raise SystemExit(r.returncode or 1)
    emit(list_pulls())
    raise SystemExit(0)

number = str(existing.get("number") or "")
args = ["pulls", "edit", number, "--title", title]
if body:
    args += ["--description", body]
r = tea(*args)
if r.returncode != 0:
    sys.stderr.write(r.stderr or r.stdout or "tea edit failed\n")
    raise SystemExit(r.returncode or 1)
emit(list_pulls())
' "${1:-}"
