---
name: flmw
description: Block distracting sites and apps for the user with the flmw CLI (fuck let me work). Use when the user asks to focus, block or unblock a site or app, start or stop a focus session, or set a focus schedule.
---

# flmw (fuck let me work)

`flmw` blocks websites (through the hosts file) and closes apps while focus is on.
A background service does the work. You only talk to it through the CLI.

## Rules

- Add `--json` to any command below to get the full status as JSON. Parse that, not the text output.
- Every command prints the new status. Check `active` to confirm the result.
- Turn focus off (`flmw off`) only when the user asks for it. Never turn it off to reach a blocked site for your own work.
- If a command fails with "daemon not reachable", tell the user to run `sudo flmw install`. Do not run sudo yourself.

## Commands

```sh
flmw status --json                       # current state
flmw on 25m --json                       # focus for 25 minutes (any Go duration: 90m, 2h)
flmw on --json                           # focus until "flmw off"
flmw off --json                          # stop focus; inside a schedule window it pauses until the window ends
flmw site add youtube.com reddit.com --json
flmw site rm youtube.com --json
flmw app add Slack Discord --json        # process name, case does not matter
flmw app rm Slack --json
flmw schedule add "0 9 * * 1-5" 2h --json   # cron: minute hour day month weekday
flmw schedule rm <id> --json             # ids are in status.schedules[].id
```

## Status JSON

```json
{
  "active": true,
  "reason": "manual",            // "manual" | "schedule", missing when off
  "until": "2026-11-24T11:40:00Z", // missing when focus has no end
  "sites": ["x.com", "twitter.com"],
  "apps": ["Slack"],
  "schedules": [{"id": "s1732...", "cron": "0 9 * * 1-5", "duration": "2h"}],
  "session": {"state": "focusing", "until": "..."}  // "idle" | "focusing" | "paused"
}
```

## Notes

- Sites block the domain and `www.` only. Other subdomains (for example `m.youtube.com`) need their own `site add`.
- A browser can keep a blocked site open for up to a minute after `on`, until its DNS cache expires.
