package egress

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestPublic(t *testing.T) {
	for ip, want := range map[string]bool{
		"8.8.8.8": true, "2606:4700::1111": true, "127.0.0.1": false, "10.1.2.3": false, "172.16.0.1": false,
		"192.168.1.1": false, "169.254.169.254": false, "::1": false, "fe80::1": false, "fd00::1": false,
		"::ffff:127.0.0.1": false, "0.0.0.0": false, "100.64.1.1": false, "fd00:ec2::254": false,
	} {
		if got := Public(netip.MustParseAddr(ip)); got != want {
			t.Errorf("Public(%s) = %v", ip, got)
		}
	}
}

func TestCheckURL(t *testing.T) {
	for _, bad := range []string{"ftp://x", "file:///etc/passwd", "http://", "javascript:alert(1)", "http://u:p@h/"} {
		if _, err := CheckURL(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	u, err := CheckURL("https://discord.com/api/webhooks/1/x")
	if err != nil || PreCheck(u, false) != nil {
		t.Fatal(err)
	}
	u, _ = CheckURL("http://127.0.0.1:11434")
	if PreCheck(u, false) == nil || PreCheck(u, true) != nil {
		t.Fatal("precheck")
	}
}

func TestDialGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	if _, err := Client(false, 5*time.Second).Get(srv.URL); !IsForbidden(err) {
		t.Fatalf("loopback not refused: %v", err)
	}
	resp, err := Client(true, 5*time.Second).Get(srv.URL)
	if err != nil || resp.StatusCode != 204 {
		t.Fatalf("allowed client: %v", err)
	}
	resp.Body.Close()
	// a public-looking redirect target that resolves to loopback is refused at dial time too
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://localhost:1/", http.StatusFound)
	}))
	defer redir.Close()
	if _, err := Client(false, 5*time.Second).Get(redir.URL); !IsForbidden(err) {
		t.Fatalf("redirect: %v", err)
	}
}
