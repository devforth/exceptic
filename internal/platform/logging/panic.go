package logging

import (
	"fmt"
	"runtime/debug"
)

// PanicError turns the value of recover() into an error carrying the stack;
// call it in the deferred function, where the stack still shows the panic.
func PanicError(recovered any) error {
	return fmt.Errorf("panic: %v\n%s", recovered, debug.Stack())
}
