// Package ui groups everything that presents the clock's value to a human:
// plain console output, raw-mode text input (textmode), and the tview
// screen (tui), plus the logic that switches between the latter two.
package ui

import "context"

// RunConsole is plain-console mode: the clock and printer (started by the
// caller, e.g. app.Start) already do all the work in the background, so
// this just waits for ctx to be cancelled (e.g. Ctrl-C).
func RunConsole(ctx context.Context) {
	<-ctx.Done()
}
