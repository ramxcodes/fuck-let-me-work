package focus

import (
	"testing"
	"time"
)

// Monday 2026-11-23 10:00 local time.
var monday10 = time.Date(2026, 11, 23, 10, 0, 0, 0, time.Local)

func weekdayMornings(t *testing.T) Config {
	t.Helper()
	c, _, err := AddSchedule(Default(), monday10, "0 9 * * 1-5", 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEvaluateSchedules(t *testing.T) {
	c := weekdayMornings(t)
	cases := []struct {
		name   string
		now    time.Time
		active bool
	}{
		{"inside window", monday10, true},
		{"window start", monday10.Add(-time.Hour), true},
		{"before window", monday10.Add(-time.Hour - time.Minute), false},
		{"window end is exclusive", monday10.Add(time.Hour), false},
		{"sunday", monday10.Add(-24 * time.Hour), false},
	}
	for _, tc := range cases {
		st := Evaluate(c, tc.now)
		if st.Active != tc.active {
			t.Errorf("%s: active=%v, want %v", tc.name, st.Active, tc.active)
		}
	}
	st := Evaluate(c, monday10)
	if st.Reason != ReasonSchedule || !st.Until.Equal(monday10.Add(time.Hour)) {
		t.Errorf("reason=%q until=%v", st.Reason, st.Until)
	}
}

func TestOverlappingWindowsUseTheLatestEnd(t *testing.T) {
	c, _, err := AddSchedule(Default(), monday10, "0 * * * *", 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// At 10:30 the 9:00 window ends at 11:00, but the 10:00 window runs to 12:00.
	st := Evaluate(c, monday10.Add(30*time.Minute))
	if want := monday10.Add(2 * time.Hour); st.Until == nil || !st.Until.Equal(want) {
		t.Fatalf("until = %v, want %v", st.Until, want)
	}
}

func TestStopInsideScheduleWindowPausesUntilWindowEnds(t *testing.T) {
	c := Stop(weekdayMornings(t), monday10)
	if c.Session.State != Paused || !c.Session.Until.Equal(monday10.Add(time.Hour)) {
		t.Fatalf("session = %+v", c.Session)
	}
	if Evaluate(c, monday10.Add(30*time.Minute)).Active {
		t.Error("still blocking while paused")
	}
	// Next day the pause is gone and the schedule works again.
	if !Evaluate(c, monday10.Add(24*time.Hour)).Active {
		t.Error("schedule did not come back after pause ended")
	}
}

func TestStopOutsideWindowIsIdle(t *testing.T) {
	on, _ := Start(Default(), monday10, 0)
	c := Stop(on, monday10)
	if c.Session.State != Idle || c.Session.Until != nil {
		t.Fatalf("session = %+v", c.Session)
	}
}

func TestManualSessionExpires(t *testing.T) {
	c, _ := Start(Default(), monday10, 25*time.Minute)
	if st := Evaluate(c, monday10.Add(24*time.Minute)); !st.Active || st.Reason != ReasonManual {
		t.Fatalf("want manual active, got %+v", st)
	}
	st := Evaluate(c, monday10.Add(25*time.Minute))
	if st.Active || st.Session.State != Idle {
		t.Fatalf("want idle after expiry, got %+v", st)
	}
}

func TestStartWithoutDurationRunsUntilStopped(t *testing.T) {
	c, _ := Start(Default(), monday10, 0)
	if !Evaluate(c, monday10.Add(1000*time.Hour)).Active {
		t.Fatal("open session ended on its own")
	}
}

func TestNormalizeSite(t *testing.T) {
	ok := map[string]string{
		"twitter.com":                    "twitter.com",
		"https://www.Twitter.com/home":   "twitter.com",
		"WWW.linkedin.com/feed":          "linkedin.com",
		"news.ycombinator.com.":          "news.ycombinator.com",
		"http://old.reddit.com:8080/r/x": "old.reddit.com",
	}
	for in, want := range ok {
		got, err := NormalizeSite(in)
		if err != nil || got != want {
			t.Errorf("NormalizeSite(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "localhost", "not a domain.com", "127.0.0.1", "x"} {
		if _, err := NormalizeSite(bad); err == nil {
			t.Errorf("NormalizeSite(%q) should fail", bad)
		}
	}
}

func TestRejectsBadInput(t *testing.T) {
	if _, _, err := AddSchedule(Default(), monday10, "every morning", time.Hour); err == nil {
		t.Error("bad cron accepted")
	}
	if _, _, err := AddSchedule(Default(), monday10, "0 9 * * *", 0); err == nil {
		t.Error("zero duration accepted")
	}
	if _, err := Start(Default(), monday10, -time.Minute); err == nil {
		t.Error("negative duration accepted")
	}
	if _, err := RemoveSite(Default(), "example.com"); err == nil {
		t.Error("removing a missing site should fail")
	}
	if _, err := RemoveApp(Default(), "Slack"); err == nil {
		t.Error("removing a missing app should fail")
	}
}

func TestDurationString(t *testing.T) {
	for d, want := range map[time.Duration]string{
		3 * time.Hour: "3h", 90 * time.Minute: "1h30m", 45 * time.Minute: "45m",
		30 * time.Second: "30s", time.Hour + time.Second: "1h0m1s", 0: "0s",
	} {
		if got := Duration(d).String(); got != want {
			t.Errorf("%v: got %q, want %q", d, got, want)
		}
		var back Duration
		if err := back.UnmarshalText([]byte(Duration(d).String())); err != nil || time.Duration(back) != d {
			t.Errorf("%v does not round-trip: %v %v", d, back, err)
		}
	}
}
