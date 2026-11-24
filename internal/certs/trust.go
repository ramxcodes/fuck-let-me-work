package certs

import (
	"fmt"
	"os/exec"
	"strings"
)

// Trust and Untrust live in trust_<os>.go. Both need admin rights.
// They return notes for the user about steps the code could not do itself.

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
