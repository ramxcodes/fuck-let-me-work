// Package skill holds SKILL.md, the instructions `flmw skill` prints for AI agents.
package skill

import _ "embed"

//go:embed SKILL.md
var Text string
