package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// AWS published test vectors (aws-sig-v4-test-suite « get-vanilla », and the S3 « GET Object » and
// « GET Bucket (List Objects) » examples of the SigV4 documentation).
func TestSigV4KnownVectors(t *testing.T) {
	cases := []struct {
		name, method, url, secret, access, region, service, date, payload string
		headers                                                           map[string]string
		want                                                              string
	}{
		{name: "get-vanilla", method: "GET", url: "https://example.amazonaws.com/", access: "AKIDEXAMPLE",
			secret: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", region: "us-east-1", service: "service", date: "20150830T123600Z",
			payload: emptySHA256,
			want:    "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"},
		{name: "s3-get-object", method: "GET", url: "https://examplebucket.s3.amazonaws.com/test.txt", access: "AKIAIOSFODNN7EXAMPLE",
			secret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", region: "us-east-1", service: "s3", date: "20130524T000000Z", payload: emptySHA256,
			headers: map[string]string{"Range": "bytes=0-9", "X-Amz-Content-Sha256": emptySHA256},
			want:    "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"},
		{name: "s3-list-objects", method: "GET", url: "https://examplebucket.s3.amazonaws.com/?max-keys=2&prefix=J", access: "AKIAIOSFODNN7EXAMPLE",
			secret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", region: "us-east-1", service: "s3", date: "20130524T000000Z", payload: emptySHA256,
			headers: map[string]string{"X-Amz-Content-Sha256": emptySHA256},
			want:    "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"},
	}
	for _, c := range cases {
		req, _ := http.NewRequest(c.method, c.url, nil)
		req.Header.Set("X-Amz-Date", c.date)
		for k, v := range c.headers {
			req.Header.Set(k, v)
		}
		signV4(req, c.access, c.secret, c.region, c.service, c.payload)
		if got := req.Header.Get("Authorization"); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

// fakeS3 is an in-memory bucket that checks every request: signature (recomputed from what it
// received), payload hash, and the addressing style.
type fakeS3 struct {
	t         *testing.T
	bucket    string
	secret    string
	pathStyle bool
	mu        sync.Mutex
	objects   map[string][]byte
	modified  map[string]time.Time
	requests  []string
}

func newFakeS3(t *testing.T, bucket, secret string, pathStyle bool) (*fakeS3, *httptest.Server) {
	f := &fakeS3{t: t, bucket: bucket, secret: secret, pathStyle: pathStyle, objects: map[string][]byte{}, modified: map[string]time.Time{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeS3) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	sum := sha256.Sum256(body)
	if r.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(sum[:]) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "<Error><Code>XAmzContentSHA256Mismatch</Code></Error>")
		return
	}
	auth := r.Header.Get("Authorization")
	check, _ := http.NewRequest(r.Method, "http://"+r.Host+r.URL.RequestURI(), nil)
	for k, vs := range r.Header {
		if k != "Authorization" {
			check.Header[k] = vs
		}
	}
	check.Host = r.Host
	signV4(check, "AKTEST", f.secret, "us-east-1", "s3", r.Header.Get("X-Amz-Content-Sha256"))
	if auth == "" || check.Header.Get("Authorization") != auth || !strings.Contains(auth, "Credential=AKTEST/") {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "<Error><Code>SignatureDoesNotMatch</Code></Error>")
		return
	}
	host, _, _ := net.SplitHostPort(r.Host)
	path := strings.TrimPrefix(r.URL.Path, "/")
	if f.pathStyle {
		if host != "127.0.0.1" || !strings.HasPrefix(path, f.bucket) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		path = strings.TrimPrefix(strings.TrimPrefix(path, f.bucket), "/")
	} else if host != f.bucket+".s3.test" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	f.requests = append(f.requests, r.Method+" "+path)
	switch {
	case r.Method == http.MethodHead && path == "":
	case r.Method == http.MethodPut:
		f.objects[path] = body
		f.modified[path] = time.Now()
	case r.Method == http.MethodDelete:
		delete(f.objects, path)
	case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
		prefix := r.URL.Query().Get("prefix")
		var keys []string
		for k := range f.objects {
			if strings.HasPrefix(k, prefix) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult>`)
		for _, k := range keys {
			fmt.Fprintf(&b, "<Contents><Key>%s</Key><Size>%d</Size><LastModified>%s</LastModified></Contents>", k, len(f.objects[k]),
				f.modified[k].UTC().Format(time.RFC3339))
		}
		b.WriteString("<IsTruncated>false</IsTruncated></ListBucketResult>")
		_, _ = io.WriteString(w, b.String())
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

// dialTo sends every connection to addr, whatever the host (virtual-hosted buckets in tests).
func dialTo(addr string) *http.Client {
	d := &net.Dialer{Timeout: 2 * time.Second}
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return d.DialContext(ctx, network, addr)
		}}}
}

func TestS3ClientPathAndVirtualHosted(t *testing.T) {
	ctx := context.Background()
	for _, pathStyle := range []bool{true, false} {
		f, srv := newFakeS3(t, "stride-bk", "s3cr3t", pathStyle)
		u, _ := url.Parse(srv.URL)
		hc := srv.Client()
		if !pathStyle {
			u.Host = "s3.test:" + u.Port()
			hc = dialTo(srv.Listener.Addr().String())
		}
		c := &S3Client{Endpoint: u, Region: "us-east-1", Bucket: "stride-bk", AccessKey: "AKTEST", SecretKey: "s3cr3t", PathStyle: pathStyle, HTTP: hc}
		if err := c.HeadBucket(ctx); err != nil {
			t.Fatalf("pathStyle=%v head: %v", pathStyle, err)
		}
		path := t.TempDir() + "/obj"
		data := []byte("snapshot bytes")
		if err := writeFile(path, data); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		key := "prod/stride-20261107T030000Z-4.2.0.sqlite.gz"
		if err := c.Put(ctx, key, path, hex.EncodeToString(sum[:])); err != nil {
			t.Fatalf("pathStyle=%v put: %v", pathStyle, err)
		}
		if string(f.objects[key]) != string(data) {
			t.Fatalf("object not stored: %v", f.requests)
		}
		objs, err := c.List(ctx, "prod/")
		if err != nil || len(objs) != 1 || objs[0].Key != key || objs[0].Size != int64(len(data)) {
			t.Fatalf("list: %+v %v", objs, err)
		}
		if err := c.Delete(ctx, key); err != nil || len(f.objects) != 0 {
			t.Fatalf("delete: %v", err)
		}
		// A wrong secret is refused with the S3 error code, without leaking anything else.
		c.SecretKey = "wrong"
		err = c.HeadBucket(ctx)
		if se, ok := err.(*S3Error); !ok || se.Status != 403 {
			t.Fatalf("wrong secret: %v", err)
		}
		if err := c.Put(ctx, key, path, strings.Repeat("0", 64)); err == nil || strings.Contains(err.Error(), "wrong") {
			t.Fatalf("bad payload hash: %v", err)
		}
	}
}
