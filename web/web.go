// Package web holds the one HTML page (block page on blocked sites, control
// panel on localhost) and the logo it shows.
package web

import _ "embed"

//go:embed index.html
var Page []byte

//go:embed logo.png
var Logo []byte
