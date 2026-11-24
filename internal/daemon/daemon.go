// Package daemon is the root service. It owns the config, applies blocking every
// tick, and serves the page and API on localhost (HTTP and HTTPS).
package daemon

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/ramxcodes/fuck-let-me-work/internal/apps"
	"github.com/ramxcodes/fuck-let-me-work/internal/certs"
	"github.com/ramxcodes/fuck-let-me-work/internal/focus"
	"github.com/ramxcodes/fuck-let-me-work/internal/hosts"
)

type Options struct {
	DataDir    string
	HostsPath  string
	HTTPAddrs  []string
	HTTPSAddrs []string
	Page       []byte // the HTML for the block page and control panel
	Logo       []byte // PNG served at /logo.png
}

type Daemon struct {
	opt Options
	ca  *certs.Authority

	mu  sync.Mutex // guards cfg
	cfg focus.Config

	tickMu sync.Mutex // one tick at a time, so two writers never race on the hosts file
}

func (d *Daemon) configPath() string { return filepath.Join(d.opt.DataDir, "config.json") }

// Run blocks until ctx ends. On exit it removes our hosts block, so nothing stays blocked.
func Run(ctx context.Context, opt Options) error {
	if err := os.MkdirAll(opt.DataDir, 0o755); err != nil {
		return err
	}
	ca, err := certs.LoadOrCreate(opt.DataDir)
	if err != nil {
		return err
	}
	d := &Daemon{opt: opt, ca: ca}
	if err := d.load(); err != nil {
		return err
	}

	servers, err := d.listen()
	if err != nil {
		return err
	}
	d.tick()
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			d.tick()
		case <-ctx.Done():
			// Wait for running requests first, then clean up under the tick lock,
			// so a late "flmw on" cannot write the block back after cleanup.
			sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			for _, s := range servers {
				s.Shutdown(sctx)
			}
			cancel()
			// Never unlock: a request that outlived Shutdown may still wait on this
			// lock, and must not write the block back before the process exits.
			d.tickMu.Lock()
			if _, err := hosts.Apply(opt.HostsPath, nil); err != nil {
				return fmt.Errorf("clean hosts file: %w", err)
			}
			flushDNS()
			return nil
		}
	}
}

func (d *Daemon) load() error {
	b, err := os.ReadFile(d.configPath())
	if errors.Is(err, os.ErrNotExist) {
		d.cfg = focus.Default()
		return d.save()
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &d.cfg); err != nil {
		return fmt.Errorf("%s: %w", d.configPath(), err)
	}
	return nil
}

// save writes the config with a temp file and rename, so a crash never leaves half a file.
func (d *Daemon) save() error {
	b, err := json.MarshalIndent(d.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := d.configPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, d.configPath())
}

// status evaluates the config now. It also saves the session when it expired.
func (d *Daemon) status() focus.Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := focus.Evaluate(d.cfg, time.Now())
	if st.Session.State != d.cfg.Session.State {
		d.cfg.Session = st.Session
		if err := d.save(); err != nil {
			log.Printf("save config: %v", err)
		}
	}
	return st
}

// update changes the config, saves it, and applies the result at once.
func (d *Daemon) update(fn func(focus.Config, time.Time) (focus.Config, error)) (focus.Status, error) {
	d.mu.Lock()
	next, err := fn(d.cfg, time.Now())
	if err == nil {
		prev := d.cfg
		d.cfg = next
		if err = d.save(); err != nil {
			d.cfg = prev
		}
	}
	d.mu.Unlock()
	if err != nil {
		return focus.Status{}, err
	}
	return d.tick()
}

// errApply marks a change that was saved but could not be applied to the OS.
var errApply = errors.New("saved, but could not apply")

// tick makes the OS match the status: hosts block, DNS cache, and running apps.
// A hosts file error is returned (and logged), because without it no site is blocked.
func (d *Daemon) tick() (focus.Status, error) {
	d.tickMu.Lock()
	defer d.tickMu.Unlock()
	st := d.status()
	var names []string
	if st.Active {
		names = focus.Hosts(st.Sites)
	}
	changed, hostsErr := hosts.Apply(d.opt.HostsPath, names)
	if hostsErr != nil {
		hostsErr = fmt.Errorf("%w: hosts file %s: %v", errApply, d.opt.HostsPath, hostsErr)
		log.Print(hostsErr)
	}
	if changed {
		flushDNS()
		log.Printf("hosts block: %d hostnames (active=%v reason=%q)", len(names), st.Active, st.Reason)
	}
	if st.Active {
		killed, err := apps.Kill(st.Apps)
		if err != nil {
			log.Printf("kill apps: %v", err)
		}
		if len(killed) > 0 {
			log.Printf("closed apps: %v", killed)
		}
	}
	return st, hostsErr
}

// hostAllowed is true for localhost and every configured site. Requests and
// certs for any other name are refused, which stops DNS rebinding tricks.
func (d *Daemon) hostAllowed(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, h := range focus.Hosts(d.cfg.Sites) {
		if h == host {
			return true
		}
	}
	return false
}

func (d *Daemon) listen() ([]*http.Server, error) {
	handler := d.handler()
	tlsCfg := &tls.Config{GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if hello.ServerName != "" && !d.hostAllowed(hello.ServerName) {
			return nil, fmt.Errorf("not a blocked site: %s", hello.ServerName)
		}
		return d.ca.Leaf(hello.ServerName)
	}}

	var servers []*http.Server
	open := func(addrs []string, secure bool) error {
		opened := 0
		for _, addr := range addrs {
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				log.Printf("listen %s: %v", addr, err) // e.g. no IPv6; the other address may still work
				continue
			}
			s := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
			if secure {
				s.TLSConfig = tlsCfg
				ln = tls.NewListener(ln, tlsCfg)
			}
			servers = append(servers, s)
			go s.Serve(ln)
			log.Printf("serving on %s (tls=%v)", addr, secure)
			opened++
		}
		if opened == 0 && len(addrs) > 0 {
			return fmt.Errorf("could not listen on any of %v", addrs)
		}
		return nil
	}
	if err := open(d.opt.HTTPAddrs, false); err != nil {
		return nil, err
	}
	if err := open(d.opt.HTTPSAddrs, true); err != nil {
		for _, s := range servers {
			s.Close()
		}
		return nil, err
	}
	return servers, nil
}

var flushCommands = map[string][][]string{
	"darwin":  {{"dscacheutil", "-flushcache"}, {"killall", "-HUP", "mDNSResponder"}},
	"windows": {{"ipconfig", "/flushdns"}},
	"linux":   {{"resolvectl", "flush-caches"}},
}

// flushDNS clears the OS DNS cache so a hosts change works at once.
// Best effort: Linux without systemd-resolved has no cache to flush.
func flushDNS() {
	for _, c := range flushCommands[runtime.GOOS] {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		if out, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			log.Printf("flush DNS (%v): %v %s", c, err, out)
		}
	}
}
