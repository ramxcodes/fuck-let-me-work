// Package web holds the one HTML page: block page on blocked sites, control panel on localhost.
package web

import _ "embed"

//go:embed index.html
var Page []byte
