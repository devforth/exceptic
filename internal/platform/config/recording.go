package config

import (
	"fmt"
	"slices"
	"strings"
)

// Recording: which log levels are stored (RECORD_LOG_LEVELS); logs of other
// levels are dropped at ingest. A nil list means every level. Events of every
// level are stored: issues group them, and notifications filter by level.
type Recording struct {
	LogLevels []string
}

// Levels are the event levels from lowest to highest, as Sentry SDKs send them.
var Levels = []string{"debug", "info", "warning", "error", "fatal"}

// LogLevels are the log levels of the Sentry logs protocol, lowest first.
var LogLevels = []string{"trace", "debug", "info", "warn", "error", "fatal"}

// HighestFirst lists levels for display: Levels and LogLevels keep them
// lowest first.
func HighestFirst(levels []string) []string {
	reversed := slices.Clone(levels)
	slices.Reverse(reversed)

	return reversed
}

// InLevelOrder orders levels highest first, as the known list does; nil
// (every level) stays nil.
func InLevelOrder(levels, known []string) []string {
	if levels == nil {
		return nil
	}

	return slices.DeleteFunc(HighestFirst(known), func(level string) bool { return !slices.Contains(levels, level) })
}

// NotifyLevels are the event levels a new notification connection sends:
// errors, not messages or warnings.
var NotifyLevels = []string{"fatal", "error"}

func parseRecording(getenv func(string) string) (Recording, error) {
	value := strings.TrimSpace(getenv("RECORD_LOG_LEVELS"))

	if value == "" {
		return Recording{}, nil
	}

	levels, err := ParseLevels(value, LogLevels)

	if err != nil {
		return Recording{}, fmt.Errorf("RECORD_LOG_LEVELS: %w", err)
	}

	return Recording{LogLevels: levels}, nil
}

// ParseLevels reads "error,fatal" against the known levels; "all" is nil
// (every level), "none" an empty list.
func ParseLevels(value string, known []string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "all":
		return nil, nil
	case "none":
		return []string{}, nil
	}

	levels := []string{}

	for entry := range strings.SplitSeq(value, ",") {
		level := strings.ToLower(strings.TrimSpace(entry))

		if !slices.Contains(known, level) {
			return nil, fmt.Errorf("unknown level %q (expected %s, all or none)", level, strings.Join(known, ", "))
		}

		if !slices.Contains(levels, level) {
			levels = append(levels, level)
		}
	}

	return levels, nil
}

// Records reports whether a level is in the list; nil records every level.
func Records(levels []string, level string) bool {
	return levels == nil || slices.Contains(levels, level)
}

// LogLevelsFor gives the log levels a project records: its own list
// (Project.LogLevels), or nil for the default (RECORD_LOG_LEVELS).
func (recording Recording) LogLevelsFor(own *[]string) []string {
	if own != nil {
		return *own
	}

	return recording.LogLevels
}

// RecordedLogLevels are the log levels a project records (its own list or
// the default) as an explicit list, highest first: every level when nil.
func (recording Recording) RecordedLogLevels(own *[]string) []string {
	levels := recording.LogLevelsFor(own)

	if levels == nil {
		levels = LogLevels
	}

	return InLevelOrder(levels, LogLevels)
}

// EventLevelAliases are the other names SDKs give event levels (log levels
// among them), each with the one of Levels it means.
var EventLevelAliases = map[string]string{"critical": "fatal", "warn": "warning", "log": "info", "trace": "debug"}

// EventLevel maps a level to one of Levels through EventLevelAliases;
// anything else unknown counts as error. Ingest stores events with it, so
// the database and everything reading it know only these names.
func EventLevel(level string) string {
	level = strings.ToLower(level)

	if alias, ok := EventLevelAliases[level]; ok {
		return alias
	}

	if slices.Contains(Levels, level) {
		return level
	}

	return "error"
}

// ShownLevel is the event level whose color an event or log level is shown
// in, the same in the UI and in emails: one of Levels below fatal, which
// shows as error.
func ShownLevel(level string) string {
	if level = EventLevel(level); level == "fatal" {
		return "error"
	}

	return level
}

// LogLevel maps a log level to one of LogLevels ("warning" is warn,
// "critical" fatal; anything else unknown counts as info).
func LogLevel(level string) string {
	return LogLevelOf(level, 0)
}

// LogLevelOf is the level a log is stored under: its level when known, else
// the OpenTelemetry range of its severity number (1-4 trace, 5-8 debug, …,
// 21-24 fatal), else info.
func LogLevelOf(level string, severityNumber int) string {
	switch level = strings.ToLower(level); level {
	case "warning":
		return "warn"
	case "critical":
		return "fatal"
	}

	if slices.Contains(LogLevels, level) {
		return level
	}

	if severityNumber >= 1 && severityNumber <= 4*len(LogLevels) {
		return LogLevels[(severityNumber-1)/4]
	}

	return "info"
}
