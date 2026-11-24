// Package hosts keeps one marked block in the OS hosts file and never touches other lines.
package hosts

import (
	"errors"
	"os"
	"runtime"
	"strings"
)

const (
	begin = "# >>> fuck-let-me-work >>>"
	end   = "# <<< fuck-let-me-work <<<"
)

// DefaultPath is the OS hosts file.
func DefaultPath() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("SystemRoot") + `\System32\drivers\etc\hosts`
	}
	return "/etc/hosts"
}

// Render returns current with our block replaced. No hostnames means no block.
// Lines outside the block keep their exact bytes, so add then remove gives back
// the same file (one exception: a file with no final newline gets one).
func Render(current string, hostnames []string) (string, error) {
	out, err := strip(current)
	if err != nil || len(hostnames) == 0 {
		return out, err
	}
	nl := "\n"
	if strings.Contains(current, "\r\n") {
		nl = "\r\n"
	}
	var b strings.Builder
	b.WriteString(out)
	if out != "" && !strings.HasSuffix(out, "\n") {
		b.WriteString(nl)
	}
	b.WriteString(begin + nl)
	for _, h := range hostnames {
		b.WriteString("127.0.0.1 " + h + nl + "::1 " + h + nl)
	}
	b.WriteString(end + nl)
	return b.String(), nil
}

// strip removes every marked block, markers included.
func strip(s string) (string, error) {
	for {
		lines := strings.SplitAfter(s, "\n") // each line keeps its own line ending
		start, stop := -1, -1
		for i, l := range lines {
			switch strings.TrimSpace(l) {
			case begin:
				if start == -1 {
					start = i
				}
			case end:
				if start != -1 && stop == -1 {
					stop = i
				}
			}
		}
		if start == -1 {
			return s, nil
		}
		if stop == -1 {
			return s, errors.New("hosts file has a fuck-let-me-work start marker but no end marker; fix it by hand")
		}
		s = strings.Join(lines[:start], "") + strings.Join(lines[stop+1:], "")
	}
}

// Apply writes our block to the file at path. It returns true when the file changed.
// It writes in place (not rename) so file owner, mode and symlinks stay the same.
func Apply(path string, hostnames []string) (bool, error) {
	cur, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	next, err := Render(string(cur), hostnames)
	if err != nil || next == string(cur) {
		return false, err
	}
	return true, os.WriteFile(path, []byte(next), 0o644)
}
