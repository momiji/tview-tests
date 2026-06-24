// Package app wires the clock, printer, text mode and UI mode together and
// owns the mode-switching logic described in ARCHITECTURE.md.
package app

import (
	"context"
	"fmt"
	"time"

	"test/internal/clock"
	"test/internal/printer"
	"test/internal/textmode"
	"test/internal/tui"
)

// tickInterval is how often the current time is refreshed and printed.
const tickInterval = 2 * time.Second

// RunConsole is plain-console mode: print the current time every tick
// until ctx is cancelled (e.g. Ctrl-C).
func RunConsole(ctx context.Context) {
	clk := clock.New(tickInterval)
	p := printer.New()

	clk.Run(ctx, p)
}

// RunUI drives --ui mode. It starts in text mode, switches to the tview UI
// automatically, and lets the user toggle between the two with the
// spacebar. Pressing q/Q/Ctrl-C quits from either mode; because tview's
// Stop() and textmode's terminal restore both run before this function
// returns, the terminal is always left back in plain console state.
func RunUI(ctx context.Context) error {
	clk := clock.New(tickInterval)
	p := printer.New()
	go clk.Run(ctx, p)

	fmt.Println("tview-tests --ui: starting in text mode, switching to UI mode...")
	p.Disable() // entering UI mode automatically; plain printing stops here.

	for {
		sig, err := tui.Run(clk)
		if err != nil {
			return err
		}
		if sig == tui.Quit {
			return nil
		}

		// User pressed space in the UI: back to text mode.
		p.Enable()
		textSig, err := textmode.Run()
		p.Disable()
		if err != nil {
			return err
		}
		if textSig == textmode.Quit {
			return nil
		}
		// User pressed space in text mode: loop back into UI mode.
	}
}
