package garminbridge

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // OAuth1 mandates HMAC-SHA1, not a security choice of ours
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// OAuth1 HMAC-SHA1 signing, the scheme Garmin's `oauth-service/oauth/exchange/user/2.0` expects.
// Only what the exchange needs: no token secret (the request is consumer-signed), no authorization
// header parsing.

// oauth1Header builds the `Authorization: OAuth …` value for body/query params.
func oauth1Header(method, baseURL string, params url.Values, consumerKey, consumerSecret, tokenKey, tokenSecret string, now time.Time) string {
	oauth := url.Values{}
	oauth.Set("oauth_consumer_key", consumerKey)
	if tokenKey != "" {
		// The resource-owner token: absent on the consumer-signed hops (preauthorized), present on
		// the user-signed ones (exchange and refresh).
		oauth.Set("oauth_token", tokenKey)
	}
	oauth.Set("oauth_signature_method", "HMAC-SHA1")
	oauth.Set("oauth_timestamp", strconv.FormatInt(now.Unix(), 10))
	oauth.Set("oauth_nonce", nonce())
	oauth.Set("oauth_version", "1.0")

	sig := signHMACSHA1(method, baseURL, params, oauth, consumerSecret, tokenSecret)
	oauth.Set("oauth_signature", sig)

	// The header lists the oauth_* parameters only, sorted by name, values percent-encoded.
	keys := make([]string, 0, len(oauth))
	for k := range oauth {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+`="`+percentEncode(oauth.Get(k))+`"`)
	}
	return "OAuth " + strings.Join(parts, ", ")
}

// signHMACSHA1 implements RFC 5849 §3.4: normalize the parameters, build the signature base string,
// then HMAC-SHA1 with the consumer secret followed by the token secret.
func signHMACSHA1(method, baseURL string, params, oauth url.Values, consumerSecret, tokenSecret string) string {
	type pair struct{ k, v string }
	var pairs []pair
	add := func(values url.Values) {
		for k, vs := range values {
			for _, v := range vs {
				pairs = append(pairs, pair{percentEncode(k), percentEncode(v)})
			}
		}
	}
	add(params)
	add(oauth)
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k != pairs[j].k {
			return pairs[i].k < pairs[j].k
		}
		return pairs[i].v < pairs[j].v
	})
	normalized := make([]string, len(pairs))
	for i, p := range pairs {
		normalized[i] = p.k + "=" + p.v
	}
	base := strings.ToUpper(method) + "&" + percentEncode(baseURL) + "&" + percentEncode(strings.Join(normalized, "&"))
	key := percentEncode(consumerSecret) + "&" + percentEncode(tokenSecret)
	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(base))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// percentEncode is RFC 3986: unreserved characters stay, everything else becomes %XX uppercase.
// url.QueryEscape is not usable here (space as `+`, `~` escaped).
func percentEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9',
			c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		default:
			b.WriteString("%")
			b.WriteString(strings.ToUpper(hex.EncodeToString([]byte{c})))
		}
	}
	return b.String()
}

func nonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely; a timestamp still gives a distinct value per request.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
