"""Rewrite links that leave the MkDocs site to GitHub blob/tree URLs.

Keeps relative links in Markdown for local browsing; at build time, maps:
  - excluded docs (e.g. specs/**) → github.com/.../blob/main/docs/...
  - paths outside docs_dir (e.g. ../internal/...) → github.com/.../blob|tree/main/...
"""

from __future__ import annotations

import re
from pathlib import PurePosixPath

import mkdocs.plugins

# Inline markdown links: ](href) or ](href "title")
_LINK_RE = re.compile(
    r"\]\("
    r"(?P<href>[^)\s]+)"
    r"(?P<title>\s+(?:\"[^\"]*\"|'[^']*'))?"
    r"\)"
)

_SKIP_PREFIXES = ("http://", "https://", "mailto:", "tel:", "//")


def _branch(config) -> str:
    # edit_uri is like "edit/main/docs/" → branch "main"
    edit_uri = (config.get("edit_uri") or "edit/main/").strip("/")
    parts = edit_uri.split("/")
    return parts[1] if len(parts) >= 2 else "main"


def _repo_root_url(config) -> str:
    return (config.get("repo_url") or "").rstrip("/")


def _split_fragment(href: str) -> tuple[str, str]:
    if "#" not in href:
        return href, ""
    path, frag = href.split("#", 1)
    return path, f"#{frag}"


def _normalize(path: PurePosixPath) -> PurePosixPath | None:
    parts: list[str] = []
    for part in path.parts:
        if part in ("", "."):
            continue
        if part == "..":
            if not parts:
                return None
            parts.pop()
            continue
        parts.append(part)
    return PurePosixPath(*parts) if parts else PurePosixPath(".")


def _resolve_repo_path(page_src_uri: str, href_path: str) -> PurePosixPath | None:
    """Resolve a relative href to a normalized path from the repo root."""
    if not href_path or href_path.startswith("/"):
        return None
    page_dir = PurePosixPath(page_src_uri).parent
    raw = PurePosixPath("docs") / page_dir / href_path
    return _normalize(raw)


def _is_site_page(repo_path: PurePosixPath, files) -> bool:
    """True when the target is included in the MkDocs build (page or asset)."""
    if repo_path.parts[:1] != ("docs",):
        return False
    site_uri = PurePosixPath(*repo_path.parts[1:]).as_posix()
    f = files.get_file_from_path(site_uri)
    # exclude_docs keeps File entries with InclusionLevel.EXCLUDED
    return f is not None and f.inclusion.is_included()


def _github_url(config, repo_path: PurePosixPath, *, is_dir: bool, fragment: str) -> str:
    root = _repo_root_url(config)
    kind = "tree" if is_dir else "blob"
    posix = repo_path.as_posix().rstrip("/")
    return f"{root}/{kind}/{_branch(config)}/{posix}{fragment}"


@mkdocs.plugins.event_priority(50)
def on_page_markdown(markdown, page, config, files):
    if not _repo_root_url(config):
        return markdown

    src = page.file.src_uri

    def repl(match: re.Match) -> str:
        href = match.group("href")
        title = match.group("title") or ""
        if not href or href.startswith("#"):
            return match.group(0)
        if any(href.lower().startswith(p) for p in _SKIP_PREFIXES):
            return match.group(0)

        path_part, fragment = _split_fragment(href)
        repo_path = _resolve_repo_path(src, path_part)
        if repo_path is None or repo_path == PurePosixPath("."):
            return match.group(0)

        is_dir = path_part.endswith("/") or repo_path.suffix == ""
        check = PurePosixPath(repo_path.as_posix().rstrip("/"))
        if _is_site_page(check, files):
            return match.group(0)

        url = _github_url(config, check, is_dir=is_dir, fragment=fragment)
        return f"]({url}{title})"

    return _LINK_RE.sub(repl, markdown)
