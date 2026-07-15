// Package rbrowser embeds the client assets so the server ships as a single
// binary (go:embed can't reach parent dirs, hence this root-level package).
package rbrowser

import "embed"

//go:embed public
var Public embed.FS
