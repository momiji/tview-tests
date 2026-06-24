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

// startupTextDuration is how long --ui mode stays in text mode on startup
// before automatically switching to the tview UI.
const startupTextDuration = 3 * time.Second

// RunConsole is plain-console mode: print the current time every tick
// until ctx is cancelled (e.g. Ctrl-C).
func RunConsole(ctx context.Context) {
	clk := clock.New(tickInterval)
	p := printer.New()

	clk.Run(ctx, p)
}

// RunUI drives --ui mode. It starts in text mode and automatically
// switches to the tview UI after startupTextDuration (or sooner, if the
// user presses space first), then lets the user toggle between the two
// with the spacebar. Pressing q/Q/Ctrl-C quits from either mode; because
// tview's Stop() and textmode's terminal restore both run before this
// function returns, the terminal is always left back in plain console
// state.
func RunUI(ctx context.Context) error {
	clk := clock.New(tickInterval)
	p := printer.New()
	go clk.Run(ctx, p)

	fmt.Println("tview-tests --ui: starting in text mode, switching to UI mode in 3s (or press space)...")

	// The startup auto-switch is fired asynchronously, on its own timer
	// goroutine, rather than being a parameter textmode itself understands.
	// This stands in for some other, real trigger that could ask for the
	// switch at an arbitrary time; textmode.Run only knows it was told to
	// switch, not why.
	autoSwitch := make(chan struct{})
	go func() {
		time.Sleep(startupTextDuration)
		close(autoSwitch)
	}()

	sig, err := textmode.Run(autoSwitch)
	if err != nil {
		return err
	}
	if sig == textmode.Quit {
		return nil
	}

	for {
		p.Disable()
		uiSig, err := tui.Run(clk)
		if err != nil {
			return err
		}
		if uiSig == tui.Quit {
			return nil
		}

		// User pressed space in the UI: back to text mode. No automatic
		// switch trigger here — only a keypress brings it back to the UI.
		p.Enable()
		textSig, err := textmode.Run(nil)
		if err != nil {
			return err
		}
		if textSig == textmode.Quit {
			return nil
		}
		// User pressed space in text mode: loop back into UI mode.
	}
}
