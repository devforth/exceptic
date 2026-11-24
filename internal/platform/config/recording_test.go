package config

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestRecording(t *testing.T) {
	defaults, err := loadFrom(env(nil))

	if err != nil {
		t.Fatal(err)
	}

	if defaults.Recording.LogLevels != nil {
		t.Fatalf("defaults: %+v", defaults.Recording)
	}

	values, err := loadFrom(env(map[string]string{"RECORD_LOG_LEVELS": "Warn, error, warn"}))

	if err != nil {
		t.Fatal(err)
	}

	if fmt.Sprint(values.Recording.LogLevels) != "[warn error]" {
		t.Fatalf("values: %+v", values.Recording)
	}

	if none, _ := ParseLevels("none", LogLevels); none == nil || len(none) != 0 {
		t.Fatal(`"none" must be an empty list, not nil (which records every level)`)
	}

	if _, err := loadFrom(env(map[string]string{"RECORD_LOG_LEVELS": "warning"})); err == nil || !strings.Contains(err.Error(), "RECORD_LOG_LEVELS") {
		t.Errorf("RECORD_LOG_LEVELS=warning: %v", err)
	}
}

func TestLevelNames(t *testing.T) {
	for level, want := range map[string]string{"critical": "fatal", "warn": "warning", "WARNING": "warning", "error": "error", "trace": "debug", "log": "info", "weird": "error"} {
		if got := EventLevel(level); got != want {
			t.Errorf("EventLevel(%q) = %q, want %q", level, got, want)
		}
	}

	for level, want := range map[string]string{"warning": "warn", "critical": "fatal", "trace": "trace", "weird": "info"} {
		if got := LogLevel(level); got != want {
			t.Errorf("LogLevel(%q) = %q, want %q", level, got, want)
		}
	}

	// A known level wins; without one, the OpenTelemetry range of the severity number.
	for _, test := range []struct {
		level    string
		severity int
		want     string
	}{
		{"error", 9, "error"}, {"WARNING", 1, "warn"}, {"critical", 0, "fatal"},
		{"", 1, "trace"}, {"", 4, "trace"}, {"", 5, "debug"}, {"", 9, "info"}, {"", 12, "info"},
		{"", 13, "warn"}, {"", 17, "error"}, {"", 20, "error"}, {"weird", 21, "fatal"}, {"", 24, "fatal"},
		{"", 0, "info"}, {"", -3, "info"}, {"", 25, "info"}, {"weird", 1000, "info"},
	} {
		if got := LogLevelOf(test.level, test.severity); got != test.want {
			t.Errorf("LogLevelOf(%q, %d) = %q, want %q", test.level, test.severity, got, test.want)
		}
	}

	for level, want := range map[string]string{"fatal": "error", "critical": "error", "warn": "warning", "log": "info", "trace": "debug", "debug": "debug"} {
		if got := ShownLevel(level); got != want {
			t.Errorf("ShownLevel(%q) = %q, want %q", level, got, want)
		}
	}
}

// A project's recorded log levels as an explicit list, highest first: its
// own, else the default, else every level; an empty list stays empty.
func TestRecordedLogLevels(t *testing.T) {
	none := []string{}
	own := []string{"debug", "fatal", "error"}

	if logs := (Recording{}).RecordedLogLevels(nil); !slices.Equal(logs, HighestFirst(LogLevels)) {
		t.Errorf("every level: %v", logs)
	}

	if logs := (Recording{LogLevels: []string{"error", "warn"}}).RecordedLogLevels(nil); !slices.Equal(logs, []string{"error", "warn"}) {
		t.Errorf("default: %v", logs)
	}

	if logs := (Recording{}).RecordedLogLevels(&own); !slices.Equal(logs, []string{"fatal", "error", "debug"}) {
		t.Errorf("own: %v", logs)
	}

	if logs := (Recording{}).RecordedLogLevels(&none); logs == nil || len(logs) != 0 {
		t.Errorf("none: %v", logs)
	}
}
