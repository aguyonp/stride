// Package egress guards outbound requests to user-supplied URLs (SSRF): only http(s), and
// loopback/private/link-local/metadata addresses are refused at dial time unless explicitly allowed.
package egress

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

var ErrForbidden = errors.New("adresse de destination interdite (réseau privé ou local)")

var blocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("fd00:ec2::/32"),
}

// Public reports whether ip is a routable public unicast address.
func Public(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || !ip.IsGlobalUnicast() {
		return false
	}
	for _, p := range blocked {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// CheckURL validates the scheme and host of a user-supplied URL.
func CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return nil, errors.New("URL invalide : http(s)://hôte[:port]/… attendu")
	}
	return u, nil
}

func control(allowPrivate bool) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, _ syscall.RawConn) error {
		if allowPrivate {
			return nil
		}
		ap, err := netip.ParseAddrPort(address)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrForbidden, address)
		}
		if !Public(ap.Addr()) {
			return fmt.Errorf("%w: %s", ErrForbidden, ap.Addr())
		}
		return nil
	}
}

// Client returns an HTTP client whose every connection (redirects included) passes the dial-time check.
func Client(allowPrivate bool, timeout time.Duration) *http.Client {
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second, Control: control(allowPrivate)}
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           d.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{Transport: tr, Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("trop de redirections")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("redirection refusée")
		}
		return nil
	}}
}

// IsForbidden reports whether err comes from the dial-time guard.
func IsForbidden(err error) bool { return errors.Is(err, ErrForbidden) }

// PreCheck rejects literal non-public hosts when saving a URL; the dial-time check stays authoritative.
func PreCheck(u *url.URL, allowPrivate bool) error {
	if allowPrivate {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	if ip, err := netip.ParseAddr(host); err == nil && !Public(ip) {
		return ErrForbidden
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") {
		return ErrForbidden
	}
	return nil
}
