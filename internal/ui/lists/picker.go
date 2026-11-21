package lists

import (
	"slices"

	"github.com/devforth/exceptic/internal/platform/config"
)

// Option is a checkbox of a multi-select: a project or a level.
type Option struct {
	Name     string
	Selected bool
}

// Options marks the chosen names among all of them.
func Options(names, chosen []string) []Option {
	result := make([]Option, 0, len(names))

	for _, name := range names {
		result = append(result, Option{Name: name, Selected: slices.Contains(chosen, name)})
	}

	return result
}

// LevelOptions lists the known levels highest first, chosen when recorded
// (nil records every level).
func LevelOptions(known, recorded []string) []Option {
	levels := config.HighestFirst(known)

	if recorded == nil {
		return Options(levels, levels)
	}

	return Options(levels, recorded)
}
