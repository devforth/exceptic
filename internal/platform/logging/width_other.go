//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package logging

import "os"

// terminalWidth is unknown here: lines are not wrapped by width.
func terminalWidth(*os.File) int { return 0 }
