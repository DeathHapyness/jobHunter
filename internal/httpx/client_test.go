package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(perHost int) *Client {
	return New(Options{
		UserAgent:      "vagas-hub/0.1",
		RequestTimeout: 5 * time.Second,
		PerHost:        perHost,
		Retries:        3,
		BaseBackoff:    time.Millisecond,
	})
}

func TestRetryOn5xxThenSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.Header.Get("User-Agent") != "vagas-hub/0.1" {
			t.Errorf("missing user agent")
		}
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	resp, err := newTestClient(1).Get(context.Background(), srv.URL+"/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "ok" || calls.Load() != 3 {
		t.Fatalf("body=%q calls=%d", b, calls.Load())
	}
}

func TestNoRetryOn404(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/robots.txt" {
			calls.Add(1)
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	_, err := newTestClient(1).Get(context.Background(), srv.URL+"/x")
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 404 {
		t.Fatalf("want 404 StatusError, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("404 should not be retried, calls=%d", calls.Load())
	}
}

func TestRobotsDisallow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			io.WriteString(w, "User-agent: *\nDisallow: /private\nAllow: /private/ok\n")
			return
		}
		io.WriteString(w, "x")
	}))
	defer srv.Close()
	c := newTestClient(1)

	if _, err := c.Get(context.Background(), srv.URL+"/private/no"); !errors.Is(err, ErrDisallowed) {
		t.Fatalf("want ErrDisallowed, got %v", err)
	}
	resp, err := c.Get(context.Background(), srv.URL+"/private/ok")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestParseRobotsSpecificAgentWins(t *testing.T) {
	txt := "User-agent: *\nDisallow: /\n\nUser-agent: vagas-hub\nDisallow: /admin\n"
	r := parseRobots(strings.NewReader(txt), "vagas-hub/0.1 (x)")
	if !r.permits("/jobs") || r.permits("/admin/x") {
		t.Fatalf("specific group not applied: %+v", r)
	}
	r = parseRobots(strings.NewReader(txt), "other-bot")
	if r.permits("/jobs") {
		t.Fatal("star group not applied")
	}
}

func TestMatchLenWildcards(t *testing.T) {
	cases := []struct {
		pattern, path string
		match         bool
	}{
		{"/embed/", "/embed/x", true},
		{"/embed/", "/v1/boards", false},
		{"/*.pdf$", "/a/b.pdf", true},
		{"/*.pdf$", "/a/b.pdf?x", false},
		{"/a*c", "/abbbc/d", true},
	}
	for _, c := range cases {
		if got := matchLen(c.pattern, c.path) >= 0; got != c.match {
			t.Errorf("matchLen(%q,%q)=%v want %v", c.pattern, c.path, got, c.match)
		}
	}
}

func TestPerHostConcurrencyLimit(t *testing.T) {
	var cur, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		cur.Add(-1)
	}))
	defer srv.Close()
	c := newTestClient(2)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := c.Get(context.Background(), srv.URL+"/x")
			if err != nil {
				t.Error(err)
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()
	}
	wg.Wait()
	if peak.Load() > 2 {
		t.Fatalf("per-host limit exceeded: peak=%d", peak.Load())
	}
}
