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
	defer cancel()

	if *uiMode {
		// autoSwitch simulates a trigger external to app.RunUI that asks
		// it to move from text mode to UI mode; here it's just a timer,
		// but it could be wired to anything else that decides when to ask
		// for the switch.
		autoSwitch := make(chan struct{})
		time.AfterFunc(startupTextDuration, func() { close(autoSwitch) })

		if err := app.RunUI(ctx, autoSwitch); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	app.RunConsole(ctx)
}
