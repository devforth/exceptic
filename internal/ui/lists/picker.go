package lists

import (
	"slices"
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
