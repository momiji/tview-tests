// Package app centralizes the application's runtime: the clock and
// printer that both plain console mode and UI mode (see ui.go) are built
// on top of, created exactly once regardless of which mode is used.
package app

import (
	"context"
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
}

// Start creates the clock and printer and starts the clock ticking (and
// printing through Printer) in the background until ctx is cancelled. It
// returns immediately; the caller decides afterwards whether to layer a UI
// on top (RunUI, in ui.go) or just let it run as plain console mode
// (RunConsole, below).
func Start(ctx context.Context) *App {
	clk := clock.New(tickInterval)
	p := printer.New()
	go clk.Run(ctx, p)
	return &App{Clock: clk, Printer: p}
}

// RunConsole is plain-console mode. Start already has the clock printing
// in the background, so there is nothing left to do but wait for ctx to be
// cancelled (e.g. Ctrl-C).
func (a *App) RunConsole(ctx context.Context) {
	<-ctx.Done()
}
