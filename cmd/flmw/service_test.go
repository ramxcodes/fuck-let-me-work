package main

import (
	"net"
	"net/http"
	"testing"
	"time"
)

func TestWaitForDaemon(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens yet, like launchd right after Start
	t.Setenv("FLMW_URL", "http://"+addr)

	if err := waitForDaemon(500 * time.Millisecond); err == nil {
		t.Fatal("want error when the daemon never answers")
	}

	go func() {
		time.Sleep(400 * time.Millisecond)
		http.ListenAndServe(addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"active":false}`))
		}))
	}()
	if err := waitForDaemon(5 * time.Second); err != nil {
		t.Fatalf("daemon came up late but wait failed: %v", err)
	}
}
