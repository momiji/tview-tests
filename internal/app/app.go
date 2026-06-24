// Package app centralizes the application's runtime: the clock and
// printer that both plain console mode and UI mode (see ui.go) are built
// on top of, created exactly once regardless of which mode is used.
package app

import (
	"context"
	"sync"
	"time"

	"test/internal/clock"
	"test/internal/printer"
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
// stdout. It returns immediately; the caller decides afterwards whether to
// layer a UI on top (RunUI, in ui.go) or just let it run as plain console
// mode (RunConsole, below). Call Wait after cancelling ctx to make sure
// both have fully stopped (and the printer has drained) before exiting.
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

// RunConsole is plain-console mode. Start already has the clock printing
// in the background, so there is nothing left to do but wait for ctx to be
// cancelled (e.g. Ctrl-C).
func (a *App) RunConsole(ctx context.Context) {
	<-ctx.Done()
}
