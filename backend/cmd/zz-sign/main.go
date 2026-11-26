// Throwaway probe: which parameter set does preauthorized actually accept?
package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func enc(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		default:
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}

func header(method, base string, params url.Values, ck, cs string) string {
	o := url.Values{}
	o.Set("oauth_consumer_key", ck)
	o.Set("oauth_signature_method", "HMAC-SHA1")
	o.Set("oauth_timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	var n [16]byte
	_, _ = rand.Read(n[:])
	o.Set("oauth_nonce", hex.EncodeToString(n[:]))
	o.Set("oauth_version", "1.0")
	type pair struct{ k, v string }
	var ps []pair
	for _, vs := range []url.Values{params, o} {
		for k, vv := range vs {
			for _, v := range vv {
				ps = append(ps, pair{enc(k), enc(v)})
			}
		}
	}
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].k != ps[j].k {
			return ps[i].k < ps[j].k
		}
		return ps[i].v < ps[j].v
	})
	var norm []string
	for _, p := range ps {
		norm = append(norm, p.k+"="+p.v)
	}
	baseStr := strings.ToUpper(method) + "&" + enc(base) + "&" + enc(strings.Join(norm, "&"))
	mac := hmac.New(sha1.New, []byte(enc(cs)+"&"))
	mac.Write([]byte(baseStr))
	o.Set("oauth_signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+`="`+enc(o.Get(k))+`"`)
	}
	return "OAuth " + strings.Join(parts, ", ")
}

const ticket = "ST-00000000-0000-0000-0000-000000000000"

func try(name, endpoint string, extra map[string]string, post bool) {
	base := "https://connectapi.garmin.com/oauth-service/oauth/" + endpoint
	p := url.Values{}
	for k, v := range extra {
		p.Set(k, v)
	}
	var req *http.Request
	if post {
		req, _ = http.NewRequest(http.MethodPost, base, strings.NewReader(p.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, _ = http.NewRequest(http.MethodGet, base+"?"+p.Encode(), nil)
	}
	req.Header.Set("Authorization", header(req.Method, base, p, "fc3e99d2-118c-44b8-8ae3-03370dde24c0", "E08WAR897WEy2knn7aFBrvegVAf0AFdWBBF"))
	req.Header.Set("User-Agent", "com.garmin.android.apps.connectmobile")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("%-46s err=%v\n", name, err)
		return
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	txt := strings.TrimSpace(string(body))
	if len(txt) > 110 {
		txt = txt[:110]
	}
	fmt.Printf("%-46s %d %s\n", name, res.StatusCode, txt)
}

func main() {
	try("GET preauthorized ticket+service+accepts", "preauthorized", map[string]string{
		"ticket": ticket, "service": "https://mobile.integration.garmin.com/gcm/android", "accepts-mfa-tokens": "true",
	}, false)
	try("GET preauthorized ticket+service", "preauthorized", map[string]string{
		"ticket": ticket, "service": "https://mobile.integration.garmin.com/gcm/android",
	}, false)
	try("GET preauthorized ticket+service=embed", "preauthorized", map[string]string{
		"ticket": ticket, "service": "https://sso.garmin.com/sso/embed",
	}, false)
	try("GET preauthorized ticket+service=modern", "preauthorized", map[string]string{
		"ticket": ticket, "service": "https://connect.garmin.com/modern/",
	}, false)
	try("GET preauthorized ticket only", "preauthorized", map[string]string{"ticket": ticket}, false)
	try("POST preauthorized ticket+service", "preauthorized", map[string]string{
		"ticket": ticket, "service": "https://mobile.integration.garmin.com/gcm/android",
	}, true)
	try("GET oauth1Token?ticket+service", "oauth1Token", map[string]string{
		"ticket": ticket, "service": "https://mobile.integration.garmin.com/gcm/android",
	}, false)
	try("GET requestToken", "requestToken", map[string]string{
		"ticket": ticket, "service": "https://mobile.integration.garmin.com/gcm/android",
	}, false)
}
