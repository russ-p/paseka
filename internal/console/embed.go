package console

import "embed"

// Console SPA assets. Third-party files live under static/lib, not static/vendor:
// module zips omit any path containing "/vendor/" (golang.org/x/mod/zip), so
// go install would ship a binary whose SPA requests then miss those files.
//
//go:embed static/*
var staticFiles embed.FS

// Queen Console Next preview assets. The production bundle under next/dist is a
// build artifact and is not committed, so a source-only `go build` embeds this
// tree as it stands — next/fallback.html and nothing else — and the console
// answers /next/ with that page. Release archives and container builds compile
// the frontend first (pnpm --dir web build) and embed the real bundle; `go
// install` is not a supported delivery path for the preview. The checked-in
// fallback is therefore not a build safety net for the embed: `all:next`
// matches it, and the handler detects a missing bundle by looking for the
// static adapter's SPA page rather than by the directory existing.
//
//go:embed all:next
var nextFiles embed.FS
