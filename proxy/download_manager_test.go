package proxy

import (
	"testing"
	"time"
)

func TestParseTorboxTimestamp(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"2026-11-12T10:15:30Z", true},
		{"2026-11-12T10:15:30.123456Z", true},
		{"2026-11-12 10:15:30", true},
		{"2026-11-12 10:15:30+00:00", true},
		{"not-a-timestamp", false},
		{"", false},
	}

	for _, tc := range cases {
		_, ok := parseTorboxTimestamp(tc.raw)
		if ok != tc.want {
			t.Fatalf("parseTorboxTimestamp(%q) got ok=%v, want %v", tc.raw, ok, tc.want)
		}
	}
}

func TestEvaluateStaleDownload(t *testing.T) {
	now := time.Date(2026, 11, 12, 18, 0, 0, 0, time.UTC)
	timeout := 24 * time.Hour

	t.Run("fresh incomplete webdl is kept", func(t *testing.T) {
		created := now.Add(-6 * time.Hour).Format(time.RFC3339)
		_, remove, _ := evaluateStaleDownload("webdl", created, 1024, 0.2, 50000, true, false, false, "downloading", now, timeout, nil)
		if remove {
			t.Fatal("expected fresh webdl not to be removed")
		}
	})

	t.Run("expired incomplete webdl after 24h is removed", func(t *testing.T) {
		created := now.Add(-25 * time.Hour).Format(time.RFC3339)
		_, remove, reason := evaluateStaleDownload("webdl", created, 1024, 0.2, 0, true, false, false, "downloading", now, timeout, nil)
		if !remove || reason == "" {
			t.Fatalf("expected expired webdl to be removed with reason, got remove=%v reason=%q", remove, reason)
		}
	})

	t.Run("completed or present download is never removed", func(t *testing.T) {
		created := now.Add(-240 * time.Hour).Format(time.RFC3339)
		_, remove1, _ := evaluateStaleDownload("webdl", created, 1000, 1.0, 0, false, true, true, "completed", now, timeout, nil)
		_, remove2, _ := evaluateStaleDownload("torrent", created, 1000, 1.0, 0, false, false, true, "cached", now, timeout, nil)
		if remove1 || remove2 {
			t.Fatal("expected finished/present download never to be removed")
		}
	})

	t.Run("disabled timeout never removes", func(t *testing.T) {
		created := now.Add(-72 * time.Hour).Format(time.RFC3339)
		_, remove, _ := evaluateStaleDownload("webdl", created, 0, 0, 0, true, false, false, "stalled", now, 0, nil)
		if remove {
			t.Fatal("expected disabled timeout (0) never to remove downloads")
		}
	})

	t.Run("stalled torrent after 24h is removed", func(t *testing.T) {
		created := now.Add(-25 * time.Hour).Format(time.RFC3339)
		tracker := &downloadStallState{
			FirstSeenAt:    now.Add(-25 * time.Hour),
			LastProgressAt: now.Add(-2 * time.Hour),
			LastDownloaded: 500,
			LastProgress:   0.1,
		}
		_, remove, _ := evaluateStaleDownload("torrent", created, 500, 0.1, 0, true, false, false, "stalledDL", now, timeout, tracker)
		if !remove {
			t.Fatal("expected stalled torrent older than timeout to be removed")
		}
	})

	t.Run("actively progressing torrent survives 24h but respects 2x hard cap", func(t *testing.T) {
		created25h := now.Add(-25 * time.Hour).Format(time.RFC3339)
		tracker := &downloadStallState{
			FirstSeenAt:    now.Add(-25 * time.Hour),
			LastProgressAt: now.Add(-2 * time.Minute),
			LastDownloaded: 1000,
			LastProgress:   0.50,
		}
		tracker, remove, _ := evaluateStaleDownload("torrent", created25h, 2000, 0.55, 1024*1024, true, false, false, "downloading", now, timeout, tracker)
		if remove {
			t.Fatal("expected actively downloading torrent at 25h not to be removed")
		}

		created49h := now.Add(-49 * time.Hour).Format(time.RFC3339)
		_, removeHardCap, _ := evaluateStaleDownload("torrent", created49h, 3000, 0.60, 1024*1024, true, false, false, "downloading", now, timeout, tracker)
		if !removeHardCap {
			t.Fatal("expected torrent exceeding 2x timeout hard cap to be removed")
		}
	})

	t.Run("retry cooldown prevents hammering failed deletes", func(t *testing.T) {
		created := now.Add(-30 * time.Hour).Format(time.RFC3339)
		tracker := &downloadStallState{
			FirstSeenAt:     now.Add(-30 * time.Hour),
			LastProgressAt:  now.Add(-20 * time.Hour),
			LastDeleteTryAt: now.Add(-2 * time.Minute),
		}
		_, remove, _ := evaluateStaleDownload("webdl", created, 0, 0, 0, true, false, false, "downloading", now, timeout, tracker)
		if remove {
			t.Fatal("expected retry cooldown within 5 minutes to skip deletion")
		}
	})
}
