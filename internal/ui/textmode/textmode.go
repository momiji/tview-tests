// Package textmode reads single keypresses from the terminal without
// requiring Enter, so the plain-console UI mode can react to spacebar and
// quit keys while still behaving like a normal scrolling console.
//
// Run is implemented per-OS (see run_unix.go and run_windows.go) because
// there is no portable way to both wait on stdin with a timeout/cancel and
// keep raw-mode input working identically everywhere; see
// internal/ui/textmode/README.md for why.
package textmode

// Signal is the reason Run returned.
type Signal int

const (
	// SwitchToUI means something asked to move to the tview UI: either the
	// user pressed space, or switchSignal (see Run) fired.
	SwitchToUI Signal = iota
	// Quit means the user pressed q/Q/Ctrl-C, or the context was cancelled.
	Quit
)
