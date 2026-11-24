// Package focus holds the config and the pure rules that decide if blocking is on.
// Nothing here touches the OS, so every rule is easy to test.
package focus

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// SessionState is the manual layer on top of schedules.
//
//	idle     -> follow schedules
//	focusing -> blocking on (until Until, or forever when Until is nil)
//	paused   -> blocking off, schedules ignored until Until
//
// Allowed transitions: any state -> focusing (Start), any state -> idle or paused (Stop),
// focusing/paused -> idle when Until passes (Normalize).
type SessionState string

const (
	Idle     SessionState = "idle"
	Focusing SessionState = "focusing"
	Paused   SessionState = "paused"
)

type Session struct {
	State SessionState `json:"state"`
	Until *time.Time   `json:"until,omitempty"`
}

type Schedule struct {
	ID       string   `json:"id"`
	Cron     string   `json:"cron"`
	Duration Duration `json:"duration"`
}

type Config struct {
	Sites     []string   `json:"sites"`
	Apps      []string   `json:"apps"`
	Schedules []Schedule `json:"schedules"`
	Session   Session    `json:"session"`
}

// Reason says why blocking is on. Empty when it is off.
type Reason string

const (
	ReasonNone     Reason = ""
	ReasonManual   Reason = "manual"
	ReasonSchedule Reason = "schedule"
)

// Status is what the daemon reports to the CLI, tray, page and agents.
type Status struct {
	Active bool       `json:"active"`
	Reason Reason     `json:"reason,omitempty"`
	Until  *time.Time `json:"until,omitempty"`
	Config            // with Session already normalized
}

// Duration is a time.Duration that reads and writes as "25m", "2h".
type Duration time.Duration

func (d Duration) MarshalText() ([]byte, error) { return []byte(time.Duration(d).String()), nil }

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// Default is the config on first run. Sites and apps are only blocked while focus is on.
func Default() Config {
	return Config{
		Sites:     []string{"x.com", "twitter.com", "linkedin.com", "instagram.com"},
		Apps:      []string{},
		Schedules: []Schedule{},
		Session:   Session{State: Idle},
	}
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// scheduleWindowEnd returns the latest end of any schedule window that contains now.
func scheduleWindowEnd(schedules []Schedule, now time.Time) (time.Time, bool) {
	var end time.Time
	for _, s := range schedules {
		sched, err := cronParser.Parse(s.Cron)
		if err != nil {
			continue // validated on add; a bad hand-edit must not stop the daemon
		}
		d := time.Duration(s.Duration)
		// Every start in (now-d, now] has a window that contains now. Windows can
		// overlap ("0 * * * *" for 2h), so the real end comes from the last one.
		for start := sched.Next(now.Add(-d)); !start.After(now); start = sched.Next(start) {
			if start.Add(d).After(end) {
				end = start.Add(d)
			}
		}
	}
	return end, !end.IsZero()
}

// Normalize ends a session whose Until has passed.
func Normalize(s Session, now time.Time) Session {
	if s.State != Idle && s.Until != nil && !now.Before(*s.Until) {
		return Session{State: Idle}
	}
	if s.State == "" {
		return Session{State: Idle}
	}
	return s
}

// Evaluate decides if blocking is on right now.
func Evaluate(c Config, now time.Time) Status {
	c.Session = Normalize(c.Session, now)
	st := Status{Config: c}
	switch c.Session.State {
	case Focusing:
		st.Active, st.Reason, st.Until = true, ReasonManual, c.Session.Until
	case Paused:
		// blocking stays off
	case Idle:
		if end, ok := scheduleWindowEnd(c.Schedules, now); ok {
			st.Active, st.Reason, st.Until = true, ReasonSchedule, &end
		}
	}
	return st
}

// Start turns focus on. A zero duration means "until stopped".
func Start(c Config, now time.Time, d time.Duration) (Config, error) {
	if d < 0 {
		return c, errors.New("duration must not be negative")
	}
	c.Session = Session{State: Focusing}
	if d > 0 {
		until := now.Add(d)
		c.Session.Until = &until
	}
	return c, nil
}

// Stop turns focus off. Inside a schedule window it pauses until the window ends,
// so the schedule does not switch blocking back on two seconds later.
func Stop(c Config, now time.Time) Config {
	c.Session = Session{State: Idle}
	if end, ok := scheduleWindowEnd(c.Schedules, now); ok {
		c.Session = Session{State: Paused, Until: &end}
	}
	return c
}

var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// NormalizeSite turns "https://www.Twitter.com/home" into "twitter.com".
func NormalizeSite(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", err
		}
		s = u.Hostname()
	}
	s, _, _ = strings.Cut(s, "/")
	s = strings.TrimSuffix(strings.TrimPrefix(s, "www."), ".")
	if !domainRe.MatchString(s) {
		return "", fmt.Errorf("not a domain: %q", raw)
	}
	return s, nil
}

// Hosts lists every hostname to redirect for the sites: the domain and its www. form.
func Hosts(sites []string) []string {
	out := make([]string, 0, len(sites)*2)
	for _, s := range sites {
		out = append(out, s, "www."+s)
	}
	return out
}

func AddSite(c Config, raw string) (Config, error) {
	s, err := NormalizeSite(raw)
	if err != nil {
		return c, err
	}
	if !slices.Contains(c.Sites, s) {
		c.Sites = append(slices.Clone(c.Sites), s)
	}
	return c, nil
}

func RemoveSite(c Config, raw string) (Config, error) {
	s, err := NormalizeSite(raw)
	if err != nil {
		return c, err
	}
	if !slices.Contains(c.Sites, s) {
		return c, fmt.Errorf("site not in list: %s", s)
	}
	c.Sites = slices.DeleteFunc(slices.Clone(c.Sites), func(x string) bool { return x == s })
	return c, nil
}

func AddApp(c Config, name string) (Config, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return c, errors.New("app name is empty")
	}
	if !slices.ContainsFunc(c.Apps, func(x string) bool { return strings.EqualFold(x, name) }) {
		c.Apps = append(slices.Clone(c.Apps), name)
	}
	return c, nil
}

func RemoveApp(c Config, name string) (Config, error) {
	name = strings.TrimSpace(name)
	match := func(x string) bool { return strings.EqualFold(x, name) }
	if !slices.ContainsFunc(c.Apps, match) {
		return c, fmt.Errorf("app not in list: %s", name)
	}
	c.Apps = slices.DeleteFunc(slices.Clone(c.Apps), match)
	return c, nil
}

// AddSchedule adds a cron window, e.g. "0 9 * * 1-5" for 2h. The ID is made from now.
func AddSchedule(c Config, now time.Time, expr string, d time.Duration) (Config, Schedule, error) {
	if _, err := cronParser.Parse(expr); err != nil {
		return c, Schedule{}, fmt.Errorf("bad cron %q: %w", expr, err)
	}
	if d <= 0 {
		return c, Schedule{}, errors.New("duration must be more than 0")
	}
	s := Schedule{ID: fmt.Sprintf("s%d", now.UnixMilli()), Cron: expr, Duration: Duration(d)}
	c.Schedules = append(slices.Clone(c.Schedules), s)
	return c, s, nil
}

func RemoveSchedule(c Config, id string) (Config, error) {
	match := func(s Schedule) bool { return s.ID == id }
	if !slices.ContainsFunc(c.Schedules, match) {
		return c, fmt.Errorf("schedule not found: %s", id)
	}
	c.Schedules = slices.DeleteFunc(slices.Clone(c.Schedules), match)
	return c, nil
}
