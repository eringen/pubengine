package pubengine

import "embed"

// EmbeddedAssets contains static assets shipped with the framework:
// talkdom.js, analytics.js, dashboard.min.js
//
//go:embed embedded/*
var EmbeddedAssets embed.FS
