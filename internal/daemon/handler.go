package daemon

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ramxcodes/fuck-let-me-work/internal/api"
	"github.com/ramxcodes/fuck-let-me-work/internal/focus"
)

// change is one config edit made from a request.
type change func(r *http.Request, c focus.Config, now time.Time) (focus.Config, error)

func (d *Daemon) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, d.status())
	})
	routes := map[string]change{
		"POST /api/focus": func(r *http.Request, c focus.Config, now time.Time) (focus.Config, error) {
			var req api.FocusRequest
			if err := decode(r, &req); err != nil {
				return c, err
			}
			return focus.Start(c, now, time.Duration(req.Duration))
		},
		"DELETE /api/focus": func(_ *http.Request, c focus.Config, now time.Time) (focus.Config, error) {
			return focus.Stop(c, now), nil
		},
		"POST /api/sites": withName(focus.AddSite),
		"POST /api/apps":  withName(focus.AddApp),
		"DELETE /api/sites/{name}": func(r *http.Request, c focus.Config, _ time.Time) (focus.Config, error) {
			return focus.RemoveSite(c, r.PathValue("name"))
		},
		"DELETE /api/apps/{name}": func(r *http.Request, c focus.Config, _ time.Time) (focus.Config, error) {
			return focus.RemoveApp(c, r.PathValue("name"))
		},
		"POST /api/schedules": func(r *http.Request, c focus.Config, now time.Time) (focus.Config, error) {
			var req api.ScheduleRequest
			if err := decode(r, &req); err != nil {
				return c, err
			}
			c, _, err := focus.AddSchedule(c, now, req.Cron, time.Duration(req.Duration))
			return c, err
		},
		"DELETE /api/schedules/{id}": func(r *http.Request, c focus.Config, _ time.Time) (focus.Config, error) {
			return focus.RemoveSchedule(c, r.PathValue("id"))
		},
	}
	for pattern, fn := range routes {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			st, err := d.update(func(c focus.Config, now time.Time) (focus.Config, error) { return fn(r, c, now) })
			if err != nil {
				code := http.StatusBadRequest
				if errors.Is(err, errApply) {
					code = http.StatusInternalServerError
				}
				writeJSON(w, code, api.Error{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, st)
		})
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, api.Error{Error: "no such endpoint: " + r.Method + " " + r.URL.Path})
	})
	mux.HandleFunc("GET /logo.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=86400")
		w.Write(d.opt.Logo)
	})
	// Every other path is the page: on a blocked site it is the block page, on localhost the control panel.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(d.opt.Page)
	})
	return d.guard(mux)
}

// guard refuses unknown Host names (DNS rebinding) and API calls without the
// custom header (other websites calling the API from a browser).
func (d *Daemon) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(strings.ToLower(host), "[]")
		if !d.hostAllowed(host) {
			writeJSON(w, http.StatusForbidden, api.Error{Error: "unknown host: " + host})
			return
		}
		if !isLocalhost(host) {
			// A blocked site: never keep the connection open. Browsers reuse open
			// connections without a new DNS lookup, so a kept-alive tab would stay
			// on this page long after focus ends and the hosts file is clean.
			w.Header().Set("Connection", "close")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get(api.Header) != "1" {
			writeJSON(w, http.StatusForbidden, api.Error{Error: "missing " + api.Header + " header"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withName(fn func(focus.Config, string) (focus.Config, error)) change {
	return func(r *http.Request, c focus.Config, _ time.Time) (focus.Config, error) {
		var req api.NameRequest
		if err := decode(r, &req); err != nil {
			return c, err
		}
		return fn(c, req.Name)
	}
}

// decode reads a JSON body. An empty body leaves v at its zero value.
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return errors.New("bad JSON body: " + err.Error())
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
