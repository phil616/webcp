// Package web embeds the built administration interface in the Go executable.
package web

import "embed"

//go:embed dist
var Assets embed.FS
