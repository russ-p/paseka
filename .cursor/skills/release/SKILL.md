---
name: release
description: Cut a Paseka version release — propose semver, draft release notes, create an annotated git tag, push to trigger GoReleaser, and polish the GitHub Release notes. Use when the user asks to cut a release, bump version, tag, write release notes, or runs /release.
---

# Release

Cut a `paseka` release for this repository. GoReleaser publishes binaries on `v*` tag push; this skill handles the human steps before and after.

## Quick start

1. Preflight
2. Propose version
3. Draft release notes
4. **Wait for user confirmation**
5. Tag and push
6. Polish GitHub Release notes

## Workflow

### 1. Preflight

Run in parallel where possible:

```bash
git status --short
git branch --show-current
git fetch --tags
git tag -l 'v*' --sort=-v:refname | head -1
```

Requirements:

- On the default branch (`main`) unless the user specifies another branch
- Working tree clean (no staged/unstaged/untracked changes) — refuse unless user overrides
- Local branch not behind remote — refuse unless user overrides

### 2. Version

- Read the latest `v*` tag (e.g. `v0.1.0`)
- Propose next semver:
  - `/release` or `/release patch` → bump patch
  - `/release minor` → bump minor
  - `/release major` → bump major
  - `/release vX.Y.Z` → use exact version
- If no tags exist, start at `v0.1.0`
- Show: previous tag → proposed tag

### 3. Collect changes

```bash
git log <prev_tag>..HEAD --oneline
git log <prev_tag>..HEAD --stat
```

Also skim [`docs/plans/changelog.md`](../../../docs/plans/changelog.md) for shipped items not yet covered by a release.

Refuse an empty release (no commits since previous tag) unless the user explicitly overrides.

### 4. Draft release notes

Write in **English**. Use this template:

```markdown
## Summary

- Bullet 1 (user-facing change)
- Bullet 2

## Breaking changes

- (omit section if none)

## Docs

- (omit section if none)
```

Show the full draft to the user. **Do not tag or push until they confirm** (version + notes).

### 5. Changelog doc (optional)

If the release includes user-facing shipped work and the user wants it documented:

- Add a matching entry to [`docs/plans/changelog.md`](../../../docs/plans/changelog.md)
- Commit on the release branch **before** creating the tag
- Only commit when the user explicitly asks for a changelog update as part of the release

### 6. Tag

Create an annotated tag after confirmation:

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
```

Use the release summary (first line or short title) as the tag message when appropriate.

### 7. Push

Push only the tag (not force-push):

```bash
git push origin vX.Y.Z
```

This triggers [`.github/workflows/release.yml`](../../../.github/workflows/release.yml) → GoReleaser → GitHub Release with tar.gz assets and `checksums.txt`.

### 8. Polish GitHub Release notes

After the workflow succeeds (or once the release exists on GitHub), replace or supplement GoReleaser's auto-generated notes with the crafted draft:

```bash
gh release view vX.Y.Z
gh release edit vX.Y.Z --notes-file /tmp/release-notes.md
```

Write the notes file from the approved draft in step 4.

## Safety rules

- **Never** force-push tags
- **Never** delete or move an existing tag without explicit user request
- **Never** push a tag without showing version + notes first
- **Never** create an empty release without user override
- **Never** update git config

## Platforms

Current release targets (see [`.goreleaser.yaml`](../../../.goreleaser.yaml)):

- `linux/amd64`, `linux/arm64`
- `darwin/amd64`, `darwin/arm64`

Windows is deferred — see backlog item in [`docs/plans/backlog.md`](../../../docs/plans/backlog.md).

## Local dry-run

For config validation without publishing:

```bash
goreleaser check
goreleaser build --snapshot --clean
```

## Manual fallback

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```
