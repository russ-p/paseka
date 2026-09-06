package console

import "embed"

// Console SPA assets. Third-party files live under static/lib, not static/vendor:
// module zips omit any path containing "/vendor/" (golang.org/x/mod/zip), so
// go install would ship a binary whose SPA requests then miss those files.
//
//go:embed static/*
var staticFiles embed.FS
