package console

import "embed"

// Queen Console bundle. The production bundle under ui/dist is a build
// artifact and is not committed, so a source-only `go build` embeds this tree as
// it stands — ui/fallback.html and nothing else — and the console answers the
// root with that page. Release archives and container builds compile the
// frontend first (pnpm --dir web build) and embed the real bundle; `go install`
// is not a supported delivery path for the console. The checked-in fallback is
// therefore not a build safety net for the embed: `all:ui` matches it, and the
// handler detects a missing bundle by looking for the static adapter's SPA page
// rather than by the directory existing.
//
//go:embed all:ui
var uiFiles embed.FS
