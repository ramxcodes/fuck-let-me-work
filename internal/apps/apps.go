// Package apps closes blocked apps by killing their processes.
package apps

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

// Proc is the part of a process we match on.
type Proc struct {
	Name string // process name, e.g. "slack" or "Slack.exe"
	Exe  string // full path to the binary, may be empty
}

// Matches reports if p belongs to a blocked app name (case-insensitive).
// "Slack" matches a process named slack, Slack.exe, and on macOS anything inside Slack.app.
func Matches(p Proc, blocked string) bool {
	want := strings.ToLower(blocked)
	for _, n := range []string{p.Name, filepath.Base(p.Exe)} {
		if strings.TrimSuffix(strings.ToLower(n), ".exe") == want {
			return true
		}
	}
	return strings.Contains(strings.ToLower(p.Exe), "/"+want+".app/")
}

// Kill kills every login-user process that matches one of blocked. It returns the names it killed.
// Errors on single processes (gone already, no rights) are skipped: the next tick retries.
func Kill(blocked []string) ([]string, error) {
	if len(blocked) == 0 {
		return nil, nil
	}
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}
	self := int32(os.Getpid())
	var killed []string
	for _, pr := range procs {
		if pr.Pid <= 1 || pr.Pid == self {
			continue
		}
		name, _ := pr.Name()
		exe, _ := pr.Exe()
		p := Proc{Name: name, Exe: exe}
		for _, b := range blocked {
			if Matches(p, b) {
				if userOwned(pr) && pr.Kill() == nil {
					killed = append(killed, name)
				}
				break
			}
		}
	}
	return killed, nil
}

// userOwned is true only for processes of a normal login user. The daemon runs
// as root, so without this check "flmw app add WindowServer" (or a typo) would
// kill OS services, which often run as root or as system accounts like
// _windowserver or dbus. An unknown owner counts as not a user.
func userOwned(pr *process.Process) bool {
	if runtime.GOOS == "windows" {
		u, err := pr.Username()
		if err != nil {
			return false
		}
		u = strings.ToUpper(u)
		for _, system := range []string{`NT AUTHORITY\`, `NT SERVICE\`, `WINDOW MANAGER\`, `FONT DRIVER HOST\`, `IIS APPPOOL\`} {
			if strings.HasPrefix(u, system) {
				return false
			}
		}
		return true
	}
	uids, err := pr.Uids()
	if err != nil || len(uids) == 0 {
		return false
	}
	return LoginUID(uids[0])
}

// LoginUID reports if uid belongs to a login user: from 1000 on Linux, from 501
// on macOS. No upper limit, because systemd-homed and LDAP users have high uids.
// 65534 is "nobody".
func LoginUID(uid uint32) bool {
	first := uint32(1000)
	if runtime.GOOS == "darwin" {
		first = 501
	}
	return uid >= first && uid != 65534
}
