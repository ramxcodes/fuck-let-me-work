# fuck let me work

I open Twitter while walking to the kitchen. Then 40 minutes are gone. This app stops that.

`flmw` blocks websites and closes apps while focus is on. Open twitter.com during focus and you get a page that says "Not now." instead. It has a button to turn focus off, because sometimes you really are done.

It runs on Linux, macOS and Windows. You, the menu bar, and your AI agent can all turn it on and off.

## Install

Needs Go 1.26+.

```sh
go build -o flmw .
sudo ./flmw install
```

`install` does four things:

1. Copies the binary into a folder only admins can change (see Files below). On macOS it also links `/usr/local/bin/flmw` to it. On Windows, add that folder to your PATH.
2. Registers a background service (systemd, launchd or Windows service) that runs as root.
3. Makes a local certificate authority (CA) and adds it to your system trust, so blocked HTTPS sites show the block page and not a certificate error.
4. Starts the service.

Then start the tray app as your normal user:

```sh
flmw tray
```

On Windows, run `flmw.exe install` from an Administrator terminal.

## Use

```sh
flmw on 25m                       # focus for 25 minutes
flmw on                           # focus until you stop it
flmw off
flmw status

flmw site add youtube.com reddit.com
flmw app add Slack Discord        # process names, case does not matter
flmw schedule add "0 9 * * 1-5" 2h   # weekdays 9 to 11
flmw open                         # control panel at http://127.0.0.1
```

Add `--json` to any of these for machine-readable output.

Default blocked sites: x.com, twitter.com, linkedin.com, instagram.com. Sites and apps are blocked only while focus is on.

`flmw off` inside a schedule window pauses blocking until that window ends. Without that, the schedule would turn blocking back on two seconds later.

## Agents

```sh
flmw skill
```

prints a `SKILL.md` that tells an AI agent how to use the CLI. Save it in your agent's skills folder, for example `~/.claude/skills/flmw/SKILL.md`.

## How it works

One Go binary, four roles:

- `flmw daemon` is the root service. Every 2 seconds it checks the config and makes the OS match it:
  - it writes a marked block in the hosts file that points blocked sites at 127.0.0.1, and removes the block when focus ends or the service stops;
  - it kills processes whose name matches a blocked app (on macOS, also anything inside `Name.app`). It only kills processes of normal login users, never root, SYSTEM or other system accounts.
- The daemon serves one page on 127.0.0.1 ports 80 and 443. On a blocked site it is the block page. On `http://127.0.0.1` it is the control panel.
- For HTTPS, the daemon signs a certificate for each blocked site with the local CA when the browser asks.
- `flmw <command>` and `flmw tray` talk to the daemon over its local API.

The API needs an `X-FLMW: 1` header and only answers to localhost or a blocked site name. That stops other websites from calling it from your browser.

Files:

| | Linux | macOS | Windows |
|---|---|---|---|
| binary | `/usr/local/bin/flmw` | `/Library/Application Support/fuck-let-me-work/flmw` | `%ProgramFiles%\fuck-let-me-work\flmw.exe` |
| config and CA | `/var/lib/fuck-let-me-work` | `/Library/Application Support/fuck-let-me-work` | `%ProgramFiles%\fuck-let-me-work` |
| hosts | `/etc/hosts` | `/etc/hosts` | `%SystemRoot%\System32\drivers\etc\hosts` |

## Limits

- Only the domain and `www.` are blocked. `m.youtube.com` needs its own `site add`. The hosts file has no wildcards.
- A browser can keep a site open for about a minute after focus starts, until its DNS cache expires.
- A browser with its own DNS-over-HTTPS setting may skip the hosts file. Turn that setting off, or set it to use the system resolver.
- Chrome and Firefox on Linux keep their own certificate lists. `install` adds the CA to them when `certutil` is installed (Debian/Ubuntu: `libnss3-tools`).
- Firefox on macOS: if you see a certificate warning, set `security.enterprise_roots.enabled` to `true` in `about:config`.
- The macOS tray needs cgo, so build on a Mac to get it. A macOS binary cross-compiled from Linux has the CLI and daemon only.
- Anything on your machine can call the local API. That is the point (agents use it), but it means this stops habits, not a determined you.
- Port 80 or 443 already in use stops the daemon from starting. Check the service log.

## Uninstall

```sh
sudo flmw uninstall
```

Removes the service, the CA from your trust stores, the hosts block, the binary, and the config folder.

## Develop

```sh
go test ./...
# run the daemon on a temp hosts file and high ports, no root needed:
go run . daemon --data-dir /tmp/flmw --hosts /tmp/hosts --http 127.0.0.1:18080 --https 127.0.0.1:18443
FLMW_URL=http://127.0.0.1:18080 go run . on 25m
```
