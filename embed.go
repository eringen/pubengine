package pubengine

import "embed"

// EmbeddedAssets contains static assets shipped with the framework:
// htmx.min.js, analytics.js, dashboard.min.js
//
//go:embed embedded/*
var EmbeddedAssets embed.FS
