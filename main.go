package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"test/internal/app"
)

func main() {
	uiMode := flag.Bool("ui", false, "start in UI mode (tview); press space to toggle text/UI, q/Ctrl-C to quit")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if *uiMode {
		if err := app.RunUI(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	app.RunConsole(ctx)
}
