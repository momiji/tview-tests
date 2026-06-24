# internal/app

The orchestrator: creates the shared `Clock` and `Printer`, starts them,
runs whichever mode `cmd/tview-tests/main.go` asked for, and waits for
clean termination. All mode-specific behavior (console output, the
`--ui` switching loop, textmode, tui) lives in [internal/ui](../ui/README.md)
instead — `app` just wires things together. See
[../../ARCHITECTURE.md](../../ARCHITECTURE.md) for how this fits into the
rest of the app.

## API

- `App` holds the shared `Clock` (see
  [../service/clock/README.md](../service/clock/README.md)) and `Printer`
  (see [../service/printer/README.md](../service/printer/README.md)).
- `Start(ctx context.Context) *App` — creates them and starts **two**
  background goroutines, both tracked by an internal `sync.WaitGroup`:
  `clk.Run(ctx, p)` (ticking and queuing lines) and `p.Run(ctx)` (the
  printer's own worker, actually writing them). Returns immediately.
- `(*App).Wait()` — blocks until both of those goroutines have exited.
  Requires `ctx` to already be cancelled (or about to be), otherwise it
  blocks forever; see the shutdown sequence below.
- `(*App).Run(ctx context.Context, uiMode bool, autoSwitch <-chan struct{}) error`
  — runs a single mode to completion: `ui.RunUI(a.Clock, a.Printer,
  autoSwitch)` if `uiMode`, otherwise `ui.RunConsole(ctx)`. `autoSwitch` is
  only meaningful (and only used) when `uiMode` is set.

## How `cmd/tview-tests/main.go` drives this

`main.go` parses the `--ui` flag and a `signal.NotifyContext` for Ctrl-C,
calls `app.Start(ctx)` to get the shared `*App`, builds the external
auto-switch trigger if `--ui` was given (see
[../ui/README.md](../ui/README.md)), and calls `a.Run(ctx, *uiMode,
autoSwitch)`.

Once that call returns, `main.go` runs the shutdown sequence: `cancel()`
the context (telling `Clock.Run` and `Printer.Run` to stop) and then
`a.Wait()` (blocking until the printer has actually drained its queue and
both background goroutines have exited) *before* checking the error and
possibly calling `os.Exit`. This is what makes the printer's asynchrony
safe: without waiting, a line queued just before exit could be silently
dropped when the process terminates.
