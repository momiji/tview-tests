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
const tickInterval = 500 * time.Millisecond

// RunConsole is plain-console mode: print the current time every tick
// until ctx is cancelled (e.g. Ctrl-C).
func RunConsole(ctx context.Context) {
	clk := clock.New(tickInterval)
	p := printer.New()

	clk.Run(ctx, p)
}

// RunUI drives --ui mode. It starts in text mode and switches to the tview
// UI as soon as either the user presses space or autoSwitch fires —
// autoSwitch is owned and triggered by the caller, e.g. on a startup timer,
// so RunUI itself has no notion of why or when that happens. After the
// first switch, the user can keep toggling between text and UI mode with
// the spacebar. Pressing q/Q/Ctrl-C quits from either mode; because
// tview's Stop() and textmode's terminal restore both run before this
// function returns, the terminal is always left back in plain console
// state.
func RunUI(ctx context.Context, autoSwitch <-chan struct{}) error {
	clk := clock.New(tickInterval)
	p := printer.New()
	go clk.Run(ctx, p)

	fmt.Println("tview-tests --ui: starting in text mode, switching to UI mode automatically or on space...")

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
