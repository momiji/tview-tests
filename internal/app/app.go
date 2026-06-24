// Package app is the orchestrator: it creates the shared components
// (clock, printer), starts them, picks which UI mode to run on top of
// them, and waits for clean termination. See internal/ui for what "UI
// mode" actually means.
package app

import (
	"context"
	"sync"
	"time"

	"test/internal/service/clock"
	"test/internal/service/printer"
	"test/internal/ui"
)

// tickInterval is how often the current time is refreshed and printed.
const tickInterval = 500 * time.Millisecond

// App holds the clock and printer shared by every mode.
type App struct {
	Clock   *clock.Clock
	Printer *printer.Printer

	wg sync.WaitGroup
}

// Start creates the clock and printer and starts both running in the
// background until ctx is cancelled: the clock ticking (and queuing lines
// through Printer), and the printer's own worker actually writing them to
// stdout. It returns immediately; call Run afterwards to pick a mode, and
// Wait (after cancelling ctx) to make sure both have fully stopped (and
// the printer has drained) before exiting.
func Start(ctx context.Context) *App {
	clk := clock.New(tickInterval)
	p := printer.New()

	a := &App{Clock: clk, Printer: p}
	a.wg.Add(2)
	go func() {
		defer a.wg.Done()
		p.Run(ctx)
	}()
	go func() {
		defer a.wg.Done()
		clk.Run(ctx, p)
	}()
	return a
}

// Wait blocks until the background clock and printer workers started by
// Start have both exited. ctx must already be cancelled (or about to be)
// for this to return — otherwise it blocks forever.
func (a *App) Wait() {
	a.wg.Wait()
}

// Run picks and runs a single UI mode to completion: ui.RunUI if uiMode is
// set, otherwise ui.RunConsole. autoSwitch is only used in UI mode; pass
// nil when uiMode is false.
func (a *App) Run(ctx context.Context, uiMode bool, autoSwitch <-chan struct{}) error {
	if uiMode {
		return ui.RunUI(a.Clock, a.Printer, autoSwitch)
	}
	ui.RunConsole(ctx)
	return nil
}
