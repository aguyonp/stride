package backup

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// Minimal S3 client (stdlib only, AWS Signature Version 4): PUT object, ListObjectsV2, DELETE object
// and HEAD bucket, path-style or virtual-hosted (AWS, MinIO, R2, Scaleway, Garage, B2). Requests go
// through the egress-guarded HTTP client; secrets never appear in errors.

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// S3Client talks to one bucket.
type S3Client struct {
	Endpoint  *url.URL
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	PathStyle bool
	HTTP      *http.Client
	Now       func() time.Time
}

// S3Object is one entry of a listing.
type S3Object struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// S3Error is a non-2xx answer; Code is the S3 error code when the body carries one.
type S3Error struct {
	Status int
	Code   string
}

func (e *S3Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("S3 a répondu HTTP %d (%s)", e.Status, e.Code)
	}
	return fmt.Sprintf("S3 a répondu HTTP %d", e.Status)
}

// Retryable tells whether a failed call may succeed later (throttling, server errors, network).
func Retryable(err error) bool {
	var se *S3Error
	if errors.As(err, &se) {
		return se.Status == http.StatusTooManyRequests || se.Status >= 500
	}
	return err != nil && !errors.Is(err, context.Canceled)
}

// uriEncode is the SigV4 encoding: every byte but the unreserved characters is %XX (uppercase);
// slashes are kept when path is true.
func uriEncode(s string, path bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		case c == '/' && path:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vs := append([]string(nil), q[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, uriEncode(k, false)+"="+uriEncode(v, false))
		}
	}
	return strings.Join(parts, "&")
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// signedHeader tells which request headers are signed: host, x-amz-*, content-type, content-md5 and range.
func signedHeader(name string) bool {
	return strings.HasPrefix(name, "x-amz-") || name == "content-type" || name == "content-md5" || name == "range"
}

// signV4 sets the Authorization header of req (X-Amz-Date must already be set). The canonical URI is
// req.URL.EscapedPath() as is (S3 does not double-encode).
func signV4(req *http.Request, accessKey, secretKey, region, service, payloadHash string) {
	amzDate := req.Header.Get("X-Amz-Date")
	date := amzDate[:8]
	host := req.URL.Host
	if req.Host != "" {
		host = req.Host
	}
	headers := map[string]string{"host": host}
	for k, vs := range req.Header {
		if lk := strings.ToLower(k); signedHeader(lk) {
			for i := range vs {
				vs[i] = strings.Join(strings.Fields(vs[i]), " ")
			}
			headers[lk] = strings.Join(vs, ",")
		}
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var ch strings.Builder
	for _, k := range names {
		ch.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")
	uri := req.URL.EscapedPath()
	if uri == "" {
		uri = "/"
	}
	creq := strings.Join([]string{req.Method, uri, canonicalQuery(req.URL.Query()), ch.String(), signed, payloadHash}, "\n")
	scope := date + "/" + region + "/" + service + "/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex(creq)
	k := hmacSHA256([]byte("AWS4"+secretKey), date)
	k = hmacSHA256(k, region)
	k = hmacSHA256(k, service)
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, toSign))
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", accessKey, scope, signed, sig))
}

// objectURL addresses the bucket (key "") or an object, path-style or virtual-hosted.
func (c *S3Client) objectURL(key string) *url.URL {
	u := *c.Endpoint
	base := strings.TrimRight(u.Path, "/")
	if c.PathStyle {
		u.Path = base + "/" + c.Bucket
		if key != "" {
			u.Path += "/" + key
		}
	} else {
		u.Host = c.Bucket + "." + u.Host
		u.Path = base + "/" + key
	}
	u.RawPath = uriEncode(u.Path, true)
	u.RawQuery = ""
	return &u
}

func (c *S3Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *S3Client) do(ctx context.Context, method, key string, query url.Values, body io.Reader, size int64, payloadHash string) (*http.Response, error) {
	u := c.objectURL(key)
	if len(query) > 0 {
		u.RawQuery = canonicalQuery(query)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = size
	}
	req.Header.Set("X-Amz-Date", c.now().UTC().Format("20060102T150405Z"))
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	signV4(req, c.AccessKey, c.SecretKey, c.Region, "s3", payloadHash)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		var e struct {
			Code string `xml:"Code"`
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		_ = xml.Unmarshal(raw, &e)
		return nil, &S3Error{Status: resp.StatusCode, Code: cleanCode(e.Code)}
	}
	return resp, nil
}

// cleanCode keeps an S3 error code printable and short (it is shown to the admin).
func cleanCode(s string) string {
	s = strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
	return s[:min(len(s), 64)]
}

// Put uploads a file whose SHA-256 (hex) is known (the signed payload hash).
func (c *S3Client) Put(ctx context.Context, key, path, sha string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodPut, key, nil, f, fi.Size(), sha)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.Body.Close()
}

// List returns every object under prefix (ListObjectsV2, continuation tokens followed).
func (c *S3Client) List(ctx context.Context, prefix string) ([]S3Object, error) {
	var out []S3Object
	token := ""
	for page := 0; page < 100; page++ {
		q := url.Values{"list-type": {"2"}, "prefix": {prefix}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		resp, err := c.do(ctx, http.MethodGet, "", q, nil, 0, emptySHA256)
		if err != nil {
			return nil, err
		}
		var res struct {
			Contents []struct {
				Key          string `xml:"Key"`
				Size         int64  `xml:"Size"`
				LastModified string `xml:"LastModified"`
			} `xml:"Contents"`
			IsTruncated           bool   `xml:"IsTruncated"`
			NextContinuationToken string `xml:"NextContinuationToken"`
		}
		err = xml.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&res)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("listing S3 illisible : %w", err)
		}
		for _, o := range res.Contents {
			t, _ := time.Parse(time.RFC3339, o.LastModified)
			out = append(out, S3Object{Key: o.Key, Size: o.Size, LastModified: t})
		}
		if !res.IsTruncated || res.NextContinuationToken == "" {
			return out, nil
		}
		token = res.NextContinuationToken
	}
	return out, nil
}

// Delete removes an object (S3 answers 204 even when it does not exist).
func (c *S3Client) Delete(ctx context.Context, key string) error {
	resp, err := c.do(ctx, http.MethodDelete, key, nil, nil, 0, emptySHA256)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// HeadBucket checks that the bucket exists and the credentials may use it.
func (c *S3Client) HeadBucket(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodHead, "", nil, nil, 0, emptySHA256)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
