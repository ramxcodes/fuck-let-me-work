package apps

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// TestMain lets the test binary act as a long-running fake app.
func TestMain(m *testing.M) {
	if os.Getenv("FLMW_SLEEPER") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	os.Exit(m.Run())
}

func TestMatches(t *testing.T) {
	cases := []struct {
		p       Proc
		blocked string
		want    bool
	}{
		{Proc{Name: "slack"}, "Slack", true},
		{Proc{Name: "Slack.exe"}, "slack", true},
		{Proc{Name: "Slack Helper", Exe: "/Applications/Slack.app/Contents/Frameworks/Slack Helper"}, "Slack", true},
		{Proc{Name: "slackbot"}, "slack", false},
		{Proc{Name: "x", Exe: "/usr/bin/discord"}, "Discord", true},
		{Proc{Name: "notslack.app"}, "slack", false},
	}
	for _, c := range cases {
		if got := Matches(c.p, c.blocked); got != c.want {
			t.Errorf("Matches(%+v, %q) = %v, want %v", c.p, c.blocked, got, c.want)
		}
	}
}

func TestKillKillsRealProcess(t *testing.T) {
	if runtime.GOOS != "windows" && !LoginUID(uint32(os.Getuid())) {
		t.Skip("Kill only touches login-user processes; this test runs as a system account")
	}
	// Run a copy of this test binary under a unique name so the test cannot kill anything else.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "flmwsleeper")
	if err := os.WriteFile(bin, data, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "FLMW_SLEEPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill() })
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	if _, err := Kill([]string{"flmw-no-such-app"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		t.Fatal("killed a process that did not match")
	case <-time.After(200 * time.Millisecond):
	}

	killed, err := Kill([]string{"flmwsleeper"})
	if err != nil || len(killed) == 0 {
		t.Fatalf("killed=%v err=%v", killed, err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("process still running")
	}
}

func TestRootProcessesAreNotUserOwned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pid 1 is not init on Windows")
	}
	initProc, err := process.NewProcess(1)
	if err != nil {
		t.Skip("cannot read pid 1:", err)
	}
	if userOwned(initProc) {
		t.Fatal("pid 1 (root) treated as a user process")
	}
	self, _ := process.NewProcess(int32(os.Getpid()))
	if LoginUID(uint32(os.Getuid())) && !userOwned(self) {
		t.Fatal("own login-user process not treated as a user process")
	}
}
