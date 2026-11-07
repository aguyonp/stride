package domain

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"
)

func TestWebhookRetryable(t *testing.T) {
	dial := &url.Error{Op: "Post", URL: "https://discord.com/api/webhooks/1/tok", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}}
	read := &url.Error{Op: "Post", URL: "https://discord.com/api/webhooks/1/tok", Err: &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}}
	timeout := &url.Error{Op: "Post", URL: "https://discord.com/api/webhooks/1/tok", Err: context.DeadlineExceeded}
	cases := []struct {
		name   string
		status int
		err    error
		want   bool
	}{
		{"refused connection", 0, dial, true},
		{"429", 429, fmt.Errorf("HTTP 429"), true},
		{"503", 503, fmt.Errorf("HTTP 503"), true},
		{"404", 404, fmt.Errorf("HTTP 404"), false},
		{"timeout after send", 0, timeout, false},
		{"reset after send", 0, read, false},
	}
	for _, c := range cases {
		if got := webhookRetryable(c.status, c.err); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPublicTextNeverEchoesRawErrors(t *testing.T) {
	raw := errors.New(`Post "https://api.telegram.org/bot123:SECRET/sendMessage": EOF`)
	if msg := publicText(raw); strings.Contains(msg, "SECRET") || strings.Contains(msg, "telegram") {
		t.Fatalf("raw error leaked: %s", msg)
	}
	if msg := publicText(badRequest("Format non reconnu.")); msg != "Format non reconnu." {
		t.Fatalf("bad request message: %s", msg)
	}
	if msg := publicText(&ProviderError{Provider: "llm", Timeout: true, Err: raw}); strings.Contains(msg, "SECRET") {
		t.Fatalf("provider message leaked: %s", msg)
	}
}
