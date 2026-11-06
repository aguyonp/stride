package queue

import (
	"testing"
	"time"
)

func TestRetryDelayGrowsFromSecondsToMinutes(t *testing.T) {
	m := NewManager(Options{})
	want := []time.Duration{10 * time.Second, time.Minute, 6 * time.Minute, 15 * time.Minute, 15 * time.Minute}
	for i, w := range want {
		if got := m.retryDelay(i + 1); got != w {
			t.Fatalf("attempt %d: %s, want %s", i+1, got, w)
		}
	}
}

func TestGroupOf(t *testing.T) {
	for kind, want := range map[string]string{
		KindLLMAnalyze: GroupLLM, KindLLMPlanAdapt: GroupLLM, KindWebhookDeliver: GroupWebhook,
		KindSyncProvider: GroupSync, KindImportActivity: GroupSync, KindSyncStravaWebhook: GroupSync,
	} {
		if got := groupOf(kind); got != want {
			t.Fatalf("%s: %s, want %s", kind, got, want)
		}
	}
}
