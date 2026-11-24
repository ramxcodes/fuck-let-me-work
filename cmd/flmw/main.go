// Command flmw ("fuck let me work") blocks sites and apps while you focus.
// See `flmw help`. Agents: `flmw skill`.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ramxcodes/fuck-let-me-work/internal/api"
	"github.com/ramxcodes/fuck-let-me-work/internal/focus"
	"github.com/ramxcodes/fuck-let-me-work/internal/tray"
	"github.com/ramxcodes/fuck-let-me-work/skill"
)

const usage = `flmw - fuck let me work. Blocks sites and apps while you focus.

Focus:
  flmw on [duration]            start focus, e.g. "flmw on 25m". No duration = until "flmw off"
  flmw off                      stop focus (inside a schedule: pause until that window ends)
  flmw status                   show state, blocked sites, apps and schedules

Lists:
  flmw site add|rm <domain>...  block or unblock sites, e.g. "flmw site add youtube.com"
  flmw app add|rm <name>...     block or unblock apps by process name, e.g. "flmw app add Slack"
  flmw schedule add "<cron>" <duration>
                                block on a schedule, e.g. flmw schedule add "0 9 * * 1-5" 2h
  flmw schedule rm <id>         remove a schedule (ids are in "flmw status")

Other:
  flmw open                     open the control panel in the browser
  flmw tray                     run the menu bar / tray app
  flmw skill                    print the agent skill (how an AI agent uses this CLI)
  sudo flmw install             install the background service and trust the local CA
  sudo flmw uninstall           remove the service and the CA, clean the hosts file
  flmw daemon                   run the service in the foreground (install does this for you)

Add --json to any focus or list command for machine-readable output.
`

func main() {
	args := os.Args[1:]
	asJSON := slices.Contains(args, "--json")
	args = slices.DeleteFunc(args, func(a string) bool { return a == "--json" })
	if err := run(args, asJSON); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, asJSON bool) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	ctx := context.Background()
	c := api.NewClient()
	print := func(st focus.Status, err error) error {
		if err != nil {
			return err
		}
		return printStatus(st, asJSON)
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	case "skill":
		fmt.Print(skill.Text)
		return nil
	case "status":
		return print(c.Status(ctx))
	case "on":
		var d time.Duration
		if len(rest) > 0 {
			var err error
			if d, err = time.ParseDuration(rest[0]); err != nil || d <= 0 {
				return fmt.Errorf("bad duration %q, use e.g. 25m or 2h", rest[0])
			}
		}
		return print(c.Start(ctx, d))
	case "off":
		return print(c.Stop(ctx))
	case "site", "app":
		kind := map[string]api.Kind{"site": api.Sites, "app": api.Apps}[cmd]
		if len(rest) < 2 || (rest[0] != "add" && rest[0] != "rm") {
			return fmt.Errorf("usage: flmw %s add|rm <name>...", cmd)
		}
		var st focus.Status
		var err error
		for _, name := range rest[1:] {
			if rest[0] == "add" {
				st, err = c.Add(ctx, kind, name)
			} else {
				st, err = c.Remove(ctx, kind, name)
			}
			if err != nil {
				return err
			}
		}
		return print(st, nil)
	case "schedule":
		switch {
		case len(rest) == 3 && rest[0] == "add":
			d, err := time.ParseDuration(rest[2])
			if err != nil {
				return fmt.Errorf("bad duration %q, use e.g. 2h", rest[2])
			}
			return print(c.AddSchedule(ctx, rest[1], d))
		case len(rest) == 2 && rest[0] == "rm":
			return print(c.RemoveSchedule(ctx, rest[1]))
		}
		return errors.New(`usage: flmw schedule add "<cron>" <duration> | flmw schedule rm <id>`)
	case "open":
		tray.OpenPanel()
		return nil
	case "tray":
		tray.Run()
		return nil
	case "daemon":
		return runDaemon(rest)
	case "install":
		return install()
	case "uninstall":
		return uninstall()
	}
	return fmt.Errorf("unknown command %q, see: flmw help", cmd)
}

func printStatus(st focus.Status, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(st)
	}
	clock := func(t *time.Time) string { return t.Local().Format("15:04 Mon") }
	switch {
	case st.Active && st.Until != nil:
		fmt.Printf("Focus: ON until %s (%s)\n", clock(st.Until), st.Reason)
	case st.Active:
		fmt.Printf("Focus: ON until you run \"flmw off\" (%s)\n", st.Reason)
	case st.Session.State == focus.Paused:
		fmt.Printf("Focus: OFF, schedules paused until %s\n", clock(st.Session.Until))
	default:
		fmt.Println("Focus: OFF")
	}
	orNone := func(xs []string) string {
		if len(xs) == 0 {
			return "none"
		}
		return strings.Join(xs, ", ")
	}
	fmt.Println("Sites:", orNone(st.Sites))
	fmt.Println("Apps: ", orNone(st.Apps))
	if len(st.Schedules) == 0 {
		fmt.Println("Schedules: none")
	}
	for _, s := range st.Schedules {
		fmt.Printf("Schedule %s: %q for %s\n", s.ID, s.Cron, s.Duration)
	}
	return nil
}
