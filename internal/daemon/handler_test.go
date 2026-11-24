package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramxcodes/fuck-let-me-work/internal/api"
	"github.com/ramxcodes/fuck-let-me-work/internal/focus"
)

func newTestDaemon(t *testing.T) (*Daemon, string) {
	t.Helper()
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Daemon{opt: Options{DataDir: dir, HostsPath: hostsPath, Page: []byte("<html>page")}, cfg: focus.Default()}, hostsPath
}

func call(h http.Handler, method, host, path, body string, withHeader bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
	if withHeader {
		r.Header.Set(api.Header, "1")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestGuard(t *testing.T) {
	d, _ := newTestDaemon(t)
	h := d.handler()
	cases := []struct {
		name, host, path string
		header           bool
		code             int
	}{
		{"api from localhost", "127.0.0.1:80", "/api/status", true, 200},
		{"api without header", "127.0.0.1", "/api/status", false, 403},
		{"rebinding attacker host", "evil.example:80", "/api/status", true, 403},
		{"block page on blocked site", "www.x.com", "/home", false, 200},
		{"page on unknown host", "evil.example", "/", false, 403},
		{"ipv6 localhost", "[::1]:80", "/api/status", true, 200},
		{"unknown api path", "localhost", "/api/nope", true, 404},
		{"logo", "localhost", "/logo.png", false, 200},
	}
	for _, c := range cases {
		if got := call(h, "GET", c.host, c.path, "", c.header).Code; got != c.code {
			t.Errorf("%s: code %d, want %d", c.name, got, c.code)
		}
	}
}

func TestFocusOnWritesHostsAndOffCleansIt(t *testing.T) {
	d, hostsPath := newTestDaemon(t)
	h := d.handler()

	w := call(h, "POST", "localhost", "/api/focus", `{"duration":"25m"}`, true)
	var st focus.Status
	if err := json.NewDecoder(w.Body).Decode(&st); err != nil || w.Code != 200 {
		t.Fatalf("code %d err %v", w.Code, err)
	}
	if !st.Active || st.Reason != focus.ReasonManual || st.Until == nil {
		t.Fatalf("status = %+v", st)
	}
	b, _ := os.ReadFile(hostsPath)
	if !strings.Contains(string(b), "127.0.0.1 www.twitter.com") {
		t.Fatalf("hosts not written:\n%s", b)
	}

	// The block page turns focus off from the blocked site itself.
	if w := call(h, "DELETE", "twitter.com", "/api/focus", "", true); w.Code != 200 {
		t.Fatalf("off from block page: %d %s", w.Code, w.Body)
	}
	if b, _ := os.ReadFile(hostsPath); string(b) != "127.0.0.1 localhost\n" {
		t.Fatalf("hosts not cleaned:\n%s", b)
	}
	// Config is saved, so a restart keeps it.
	saved, _ := os.ReadFile(filepath.Join(d.opt.DataDir, "config.json"))
	if !strings.Contains(string(saved), `"state": "idle"`) {
		t.Fatalf("config not saved:\n%s", saved)
	}
}

func TestBadInputIsRejected(t *testing.T) {
	d, _ := newTestDaemon(t)
	h := d.handler()
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/sites", `{"name":"not a domain"}`},
		{"POST", "/api/schedules", `{"cron":"nope","duration":"1h"}`},
		{"POST", "/api/focus", `{"duration":"forever"}`},
		{"POST", "/api/focus", `{"surprise":1}`},
		{"DELETE", "/api/apps/Slack", ""},
	} {
		if w := call(h, c.method, "localhost", c.path, c.body, true); w.Code != 400 {
			t.Errorf("%s %s %s: code %d, want 400", c.method, c.path, c.body, w.Code)
		}
	}
}

func TestHostsErrorReachesTheCaller(t *testing.T) {
	d, hostsPath := newTestDaemon(t)
	if err := os.WriteFile(hostsPath, []byte("# >>> fuck-let-me-work >>>\nmine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := call(d.handler(), "POST", "localhost", "/api/focus", "", true)
	if w.Code != 500 || !strings.Contains(w.Body.String(), "hosts file") {
		t.Fatalf("want 500 with hosts error, got %d %s", w.Code, w.Body)
	}
}

// Browsers reuse open connections without a new DNS lookup, so a blocked
// site must never keep one open, or the tab stays here after focus ends.
func TestBlockedSiteClosesConnection(t *testing.T) {
	d, _ := newTestDaemon(t)
	h := d.handler()
	if got := call(h, "GET", "x.com", "/", "", false).Header().Get("Connection"); got != "close" {
		t.Errorf("blocked site: Connection = %q, want close", got)
	}
	if got := call(h, "GET", "x.com", "/api/status", "", true).Header().Get("Connection"); got != "close" {
		t.Errorf("blocked site API poll: Connection = %q, want close", got)
	}
	if got := call(h, "GET", "127.0.0.1", "/", "", false).Header().Get("Connection"); got != "" {
		t.Errorf("control panel: Connection = %q, want keep-alive", got)
	}
}
