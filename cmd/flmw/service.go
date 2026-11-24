package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kardianos/service"

	"github.com/ramxcodes/fuck-let-me-work/internal/api"
	"github.com/ramxcodes/fuck-let-me-work/internal/certs"
	"github.com/ramxcodes/fuck-let-me-work/internal/daemon"
	"github.com/ramxcodes/fuck-let-me-work/internal/hosts"
	"github.com/ramxcodes/fuck-let-me-work/web"
)

const appName = "fuck-let-me-work"

// defaultDataDir holds config.json and the CA (and the binary on macOS and Windows).
// Only admins can create or change files in it, so a normal user cannot plant a CA.
func defaultDataDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("ProgramFiles"), appName)
	case "darwin":
		return "/Library/Application Support/" + appName
	}
	return "/var/lib/" + appName
}

// installPath is the binary the root service runs. Its folder must be admin-only
// (copySelf checks), or a user process could swap the binary and gain root.
func installPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(defaultDataDir(), "flmw.exe")
	case "darwin":
		// Not /usr/local/bin: Homebrew on Intel Macs gives that folder to the user.
		return filepath.Join(defaultDataDir(), "flmw")
	}
	// Linux: a bin folder, because SELinux blocks services from running binaries in /var/lib.
	return "/usr/local/bin/flmw"
}

// macLink puts flmw on the PATH on macOS. A user who swaps this symlink changes
// only their own shell, not what the service runs.
const macLink = "/usr/local/bin/flmw"

func svcConfig() *service.Config {
	return &service.Config{
		Name:        appName,
		DisplayName: "Fuck Let Me Work",
		Description: "Blocks sites and apps while you focus.",
		Executable:  installPath(),
		Arguments:   []string{"daemon"},
		Option:      service.KeyValue{"RunAtLoad": true, "OnFailure": "restart"},
	}
}

type daemonFlags struct{ dataDir, hostsPath, http, https string }

// parseDaemonFlags reads the daemon flags. Defaults are the real system paths;
// tests pass a temp hosts file and high ports.
func parseDaemonFlags(args []string, opt *daemonFlags) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.StringVar(&opt.dataDir, "data-dir", opt.dataDir, "where config.json and the CA live")
	fs.StringVar(&opt.hostsPath, "hosts", opt.hostsPath, "hosts file to edit")
	fs.StringVar(&opt.http, "http", opt.http, "comma-separated HTTP listen addresses")
	fs.StringVar(&opt.https, "https", opt.https, "comma-separated HTTPS listen addresses")
	return fs.Parse(args)
}

// program adapts daemon.Run to the service manager's Start/Stop calls.
type program struct {
	opt    daemon.Options
	cancel context.CancelFunc
	done   chan error
}

func (p *program) Start(service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.done = cancel, make(chan error, 1)
	go func() {
		err := daemon.Run(ctx, p.opt)
		if err != nil && ctx.Err() == nil {
			log.Printf("daemon stopped: %v", err)
			os.Exit(1) // the service manager restarts us
		}
		p.done <- err
	}()
	return nil
}

func (p *program) Stop(service.Service) error {
	p.cancel()
	return <-p.done
}

func runDaemon(args []string) error {
	f := daemonFlags{
		dataDir:   defaultDataDir(),
		hostsPath: hosts.DefaultPath(),
		http:      "127.0.0.1:80,[::1]:80",
		https:     "127.0.0.1:443,[::1]:443",
	}
	if err := parseDaemonFlags(args, &f); err != nil {
		return err
	}
	split := func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(s, ",")
	}
	prg := &program{opt: daemon.Options{
		DataDir:    f.dataDir,
		HostsPath:  f.hostsPath,
		HTTPAddrs:  split(f.http),
		HTTPSAddrs: split(f.https),
		Page:       web.Page,
	}}
	s, err := service.New(prg, svcConfig())
	if err != nil {
		return err
	}
	return s.Run()
}

func requireAdmin() error {
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		return errors.New("needs admin rights, run: sudo flmw " + os.Args[1])
	}
	return nil // Windows: the service manager refuses with a clear error when not admin
}

func install() error {
	if err := requireAdmin(); err != nil {
		return err
	}
	s, err := service.New(&program{}, svcConfig())
	if err != nil {
		return err
	}
	// Reinstall = upgrade: stop and remove the old service first.
	if _, err := s.Status(); !errors.Is(err, service.ErrNotInstalled) {
		fmt.Println("Found an installed service, replacing it.")
		if err := s.Stop(); err != nil {
			fmt.Println("  stop:", err)
		}
		if err := s.Uninstall(); err != nil {
			return fmt.Errorf("remove old service: %w", err)
		}
	}
	if err := copySelf(installPath()); err != nil {
		return fmt.Errorf("copy binary to %s: %w", installPath(), err)
	}
	if runtime.GOOS == "darwin" {
		os.Remove(macLink) // an old binary or link from a previous install
		if err := os.MkdirAll(filepath.Dir(macLink), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(installPath(), macLink); err != nil {
			return fmt.Errorf("link %s: %w", macLink, err)
		}
	}
	if err := s.Install(); err != nil {
		return fmt.Errorf("install service: %w", err)
	}
	ca, err := certs.LoadOrCreate(defaultDataDir())
	if err != nil {
		return err
	}
	notes, err := certs.Trust(ca.CertPath)
	if err != nil {
		return fmt.Errorf("trust local CA: %w", err)
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	if err := waitForDaemon(10 * time.Second); err != nil {
		return fmt.Errorf("service installed but not answering, check its log: %w", err)
	}
	fmt.Println("Installed", installPath(), "and started the service.")
	if runtime.GOOS == "windows" {
		fmt.Println("Add", defaultDataDir(), "to your PATH to run flmw from any terminal.")
	}
	fmt.Println("Trusted the local CA", ca.CertPath)
	for _, n := range notes {
		fmt.Println("Note:", n)
	}
	fmt.Println("Next: run \"flmw tray\" for the menu bar app, or \"flmw on 25m\" to start focus.")
	return nil
}

// waitForDaemon polls the API until it answers. The service manager (launchd
// especially) starts the daemon a moment after Start returns, so a command run
// right after install would otherwise fail.
func waitForDaemon(limit time.Duration) error {
	c := api.NewClient()
	deadline := time.Now().Add(limit)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := c.Status(ctx)
		cancel()
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func uninstall() error {
	if err := requireAdmin(); err != nil {
		return err
	}
	s, err := service.New(&program{}, svcConfig())
	if err != nil {
		return err
	}
	if _, err := s.Status(); !errors.Is(err, service.ErrNotInstalled) {
		if err := s.Stop(); err != nil {
			fmt.Println("stop service:", err)
		}
		if err := s.Uninstall(); err != nil {
			return fmt.Errorf("remove service: %w", err)
		}
	}
	// The daemon cleans the hosts file when it stops. Do it again in case it crashed.
	if _, err := hosts.Apply(hosts.DefaultPath(), nil); err != nil {
		return fmt.Errorf("clean hosts file: %w", err)
	}
	notes, err := certs.Untrust(filepath.Join(defaultDataDir(), "ca.crt"))
	if err != nil {
		fmt.Println("untrust CA:", err)
	}
	for _, n := range notes {
		fmt.Println("Note:", n)
	}
	switch runtime.GOOS {
	case "darwin":
		if target, err := os.Readlink(macLink); err == nil && target == installPath() {
			os.Remove(macLink)
		}
	case "linux":
		if err := os.Remove(installPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.RemoveAll(defaultDataDir()); err != nil {
		// Windows cannot delete a running .exe, e.g. when uninstall runs from the installed copy.
		return fmt.Errorf("service removed, but delete %s by hand: %w", defaultDataDir(), err)
	}
	fmt.Println("Removed the service, the CA, the config and", installPath())
	return nil
}

// copySelf copies the running binary to dst through a temp file and rename,
// so it also works when dst is the binary that is running now.
func copySelf(dst string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if self, err = filepath.EvalSymlinks(self); err != nil {
		return err
	}
	if self == dst {
		return nil
	}
	data, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := adminOnlyDir(filepath.Dir(dst)); err != nil {
		return err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
