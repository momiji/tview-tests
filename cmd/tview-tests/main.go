package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"test/internal/app"
)

// startupTextDuration is how long --ui mode stays in text mode on startup
// before automatically switching to the tview UI.
const startupTextDuration = 3 * time.Second

func main() {
	uiMode := flag.Bool("ui", false, "start in UI mode (tview); press space to toggle text/UI, q/Ctrl-C to quit")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	a := app.Start(ctx)

	// autoSwitch simulates a trigger external to ui.RunUI that asks it to
	// move from text mode to UI mode; here it's just a timer, but it could
	// be wired to anything else that decides when to ask for the switch.
	// Only used when *uiMode is set.
	var autoSwitch chan struct{}
	if *uiMode {
		autoSwitch = make(chan struct{})
		time.AfterFunc(startupTextDuration, func() { close(autoSwitch) })
	}

	runErr := a.Run(ctx, *uiMode, autoSwitch)

	// Stop the clock/printer workers and wait for the printer to finish
	// writing anything still queued, so output isn't lost when the process
	// exits right after this.
	cancel()
	a.Wait()

	if runErr != nil {
		fmt.Fprintln(os.Stderr, "error:", runErr)
		os.Exit(1)
	}
}
