// internal/recovery/recovery.go
package recovery

import (
	"fmt"
	"os"
	"runtime/debug"
)

// HandlePanic should be deferred at the top of main() or goroutines.
// It logs panic details and exits with code 1.
func HandlePanic() {
	if r := recover(); r != nil {
		_, _ = fmt.Fprintf(os.Stderr, "FATAL: %v\n\nStack trace:\n%s\n", r, debug.Stack())
		os.Exit(1)
	}
}

// HandlePanicFunc logs panic details and calls the provided cleanup function.
func HandlePanicFunc(cleanup func()) {
	if r := recover(); r != nil {
		_, _ = fmt.Fprintf(os.Stderr, "FATAL: %v\n\nStack trace:\n%s\n", r, debug.Stack())
		if cleanup != nil {
			cleanup()
		}
		os.Exit(1)
	}
}

// Usage in goroutines (with cleanup):
//go func() {
//	defer recovery.HandlePanicFunc(func() {
//		close(d.doneCh)
//	})
//	d.processLoop(ctx)
//}()
