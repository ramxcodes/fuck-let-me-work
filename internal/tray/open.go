package tray

import (
	"log"
	"os/exec"
	"runtime"

	"github.com/ramxcodes/fuck-let-me-work/internal/api"
)

func openURL(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("open %s: %v", u, err)
	}
}

// OpenPanel opens the control panel in the default browser.
func OpenPanel() { openURL(api.DefaultURL()) }
