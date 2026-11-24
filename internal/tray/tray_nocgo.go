//go:build darwin && !cgo

package tray

import (
	"fmt"
	"os"
)

// Run explains the missing tray. The macOS tray needs cgo, so a binary
// cross-compiled from Linux or Windows has the CLI and daemon only.
func Run() {
	fmt.Fprintln(os.Stderr, "this build has no tray: build flmw on a Mac (CGO_ENABLED=1) to get the menu bar app")
	os.Exit(1)
}
