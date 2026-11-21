package helpers

import (
	"testing"
	"time"
)

func TestTimeAgo(t *testing.T) {
	now := time.Now()

	for _, test := range []struct {
		ago  time.Duration
		want string
	}{
		{59 * time.Second, "just now"},
		{time.Minute + time.Second, "1m ago"},
		{59*time.Minute + 59*time.Second, "59m ago"},
		{time.Hour + time.Second, "1h ago"},
		{24*time.Hour + time.Second, "1d ago"},
		{29 * 24 * time.Hour, "29d ago"},
	} {
		if got := TimeAgo(now.Add(-test.ago)); got != test.want {
			t.Errorf("TimeAgo(-%v) = %q, want %q", test.ago, got, test.want)
		}
	}

	// Times to come (the next retry of a message).
	for ahead, want := range map[time.Duration]string{30 * time.Second: "just now", 4*time.Minute + 10*time.Second: "in 4m", 2*time.Hour + time.Minute: "in 2h"} {
		if got := TimeAgo(now.Add(ahead)); got != want {
			t.Errorf("TimeAgo(+%v) = %q, want %q", ahead, got, want)
		}
	}

	old := now.Add(-31 * 24 * time.Hour)

	if got := TimeAgo(old); got != old.UTC().Format("2006-01-02") {
		t.Errorf("TimeAgo a month ago = %q, want the date", got)
	}
}

func TestLevelClass(t *testing.T) {
	for level, want := range map[string]string{
		"fatal": "level-error", "critical": "level-error", "warn": "level-warning", "log": "level-info", "trace": "level-debug", "debug": "level-debug",
	} {
		if got := LevelClass(level); got != want {
			t.Errorf("LevelClass(%q) = %q, want %q", level, got, want)
		}
	}
}
