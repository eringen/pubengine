// Package scaffold provides embedded template files for the pubengine CLI
// project scaffolding tool.
package scaffold

import "embed"

// Templates contains all scaffold template files.
// Files use Go text/template syntax and have a .tmpl suffix.
//
//go:embed all:templates
var Templates embed.FS
