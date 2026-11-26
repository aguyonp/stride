package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeGitHub serves a release payload and counts the calls, like the real endpoint would answer.
func fakeGitHub(t *testing.T, status int, body string) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		// The contract with GitHub: the exact path, an explicit User-Agent.
		if r.URL.Path != "/repos/aguyonp/stride/releases/latest" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func releaseBody(tag string) string {
	return `{"tag_name":"` + tag + `","published_at":"2026-11-25T10:00:00Z","prerelease":false,"draft":false,"html_url":"http://evil.example/x"}`
}

func TestVersionRules(t *testing.T) {
	for _, tc := range []struct {
		name    string
		latest  string
		current string
		newer   bool
	}{
		{"patch", "4.7.1", "4.7.0", true},
		{"minor", "4.8.0", "4.7.9", true},
		{"major", "5.0.0", "4.99.99", true},
		{"same", "4.7.0", "4.7.0", false},
		{"older", "4.6.9", "4.7.0", false},
		{"v prefix both sides", "v4.7.0", "v4.6.1", true},
		{"v prefix on tag only", "v4.7.0", "4.6.1", true},
		{"pre-release is not newer", "4.8.0-rc1", "4.7.0", false},
		{"build metadata is not newer", "4.8.0+build", "4.7.0", false},
		{"two parts", "4.7", "4.6.1", true},
		{"dev build", "4.7.0", "dev", false},
		{"garbage", "4.7.0", "main-abc", false},
		{"empty", "", "4.7.0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Newer(tc.latest, tc.current); got != tc.newer {
				t.Fatalf("Newer(%q, %q) = %v", tc.latest, tc.current, got)
			}
		})
	}
	if Comparable("dev") || Comparable("4.7.0-rc1") || !Comparable("v4.7.0") {
		t.Fatalf("Comparable")
	}
	if _, ok := normalize("4.7.0-rc1"); ok {
		t.Fatal("normalize accepted a pre-release")
	}
}

func TestCheckNewerRelease(t *testing.T) {
	srv, calls := fakeGitHub(t, 200, releaseBody("v4.7.1"))
	s := New(srv.Client(), WithBaseURL(srv.URL))

	st := s.Check(context.Background(), "4.7.0")
	if !st.Available || st.Latest != "4.7.1" || st.Current != "4.7.0" {
		t.Fatalf("status: %+v", st)
	}
	// The link comes from our constant repository and the validated tag, never from `html_url`.
	if st.URL != "https://github.com/aguyonp/stride/releases/tag/4.7.1" || strings.Contains(st.URL, "evil") {
		t.Fatalf("url: %s", st.URL)
	}
	if st.PublishedAt != "2026-11-25T10:00:00Z" || st.CheckedAt == "" {
		t.Fatalf("dates: %+v", st)
	}
	// Same version, older release, dev build: no banner.
	if st := s.Check(context.Background(), "4.7.1"); st.Available {
		t.Fatalf("same version: %+v", st)
	}
	if st := s.Check(context.Background(), "4.8.0"); st.Available {
		t.Fatalf("ahead of the release: %+v", st)
	}
	if st := s.Check(context.Background(), "dev"); st.Available || st.Latest != "4.7.1" {
		t.Fatalf("dev build: %+v", st)
	}
	// One HTTP call for all of them: the cache is shared.
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Fatalf("github calls: %d", n)
	}
	if s.Requests() != 1 {
		t.Fatalf("Requests: %d", s.Requests())
	}
}

func TestCheckIgnoresPreRelease(t *testing.T) {
	srv, _ := fakeGitHub(t, 200, `{"tag_name":"4.8.0","prerelease":true,"draft":false}`)
	s := New(srv.Client(), WithBaseURL(srv.URL))
	if st := s.Check(context.Background(), "4.7.0"); st.Available || st.Latest != "" {
		t.Fatalf("pre-release: %+v", st)
	}
}

func TestCheckFailuresAreSilentAndCached(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"rate limited", http.StatusForbidden, `{"message":"API rate limit exceeded"}`},
		{"too many requests", http.StatusTooManyRequests, `{}`},
		{"server error", http.StatusBadGateway, ``},
		{"malformed", 200, `{"tag_name":`},
		{"empty tag", 200, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, calls := fakeGitHub(t, tc.status, tc.body)
			s := New(srv.Client(), WithBaseURL(srv.URL))
			for i := 0; i < 3; i++ {
				st := s.Check(context.Background(), "4.7.0")
				// Never an error surface: no banner, no notification, just the running version.
				if st.Available || st.Latest != "" || st.URL != "" || st.Current != "4.7.0" {
					t.Fatalf("status: %+v", st)
				}
			}
			// The failure is cached: three page loads, one request.
			if n := atomic.LoadInt32(calls); n != 1 {
				t.Fatalf("github calls: %d", n)
			}
		})
	}
}

func TestCacheTTLs(t *testing.T) {
	srv, calls := fakeGitHub(t, 200, releaseBody("4.7.1"))
	now := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	s := New(srv.Client(), WithBaseURL(srv.URL), WithClock(func() time.Time { return now }))

	s.Check(context.Background(), "4.7.0")
	now = now.Add(SuccessTTL - time.Minute)
	s.Check(context.Background(), "4.7.0")
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Fatalf("within TTL: %d calls", n)
	}
	now = now.Add(2 * time.Minute)
	s.Check(context.Background(), "4.7.0")
	if n := atomic.LoadInt32(calls); n != 2 {
		t.Fatalf("after TTL: %d calls", n)
	}
}

func TestFailureTTLIsShorter(t *testing.T) {
	srv, calls := fakeGitHub(t, http.StatusInternalServerError, ``)
	now := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	s := New(srv.Client(), WithBaseURL(srv.URL), WithClock(func() time.Time { return now }))

	s.Check(context.Background(), "4.7.0")
	now = now.Add(FailureTTL - time.Minute)
	s.Check(context.Background(), "4.7.0")
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Fatalf("within failure TTL: %d calls", n)
	}
	now = now.Add(2 * time.Minute)
	s.Check(context.Background(), "4.7.0")
	if n := atomic.LoadInt32(calls); n != 2 {
		t.Fatalf("after failure TTL: %d calls", n)
	}
}

// The first check fails, the retry succeeds: the banner appears without a restart.
func TestRecoversAfterFailure(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		if fail.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(releaseBody("4.7.1")))
	}))
	defer srv.Close()
	now := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	s := New(srv.Client(), WithBaseURL(srv.URL), WithClock(func() time.Time { return now }))
	if st := s.Check(context.Background(), "4.7.0"); st.Available {
		t.Fatalf("first check: %+v", st)
	}
	fail.Store(false)
	now = now.Add(FailureTTL + time.Minute)
	if st := s.Check(context.Background(), "4.7.0"); !st.Available {
		t.Fatalf("after recovery: %+v", st)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("calls: %d", n)
	}
}
