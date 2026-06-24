# internal/app

Wires the clock, printer, text mode, and tview UI together. Split into two
files so application logic and UI logic don't mix, and so the clock and
printer are only ever created once. See
[../../ARCHITECTURE.md](../../ARCHITECTURE.md) for how this fits into the
rest of the app.

## `app.go` — shared startup and plain console mode

- `App` holds the shared `Clock` (see
  [../clock/README.md](../clock/README.md)) and `Printer` (see
  [../printer/README.md](../printer/README.md)).
- `Start(ctx context.Context) *App` — creates them and starts **two**
  background goroutines, both tracked by an internal `sync.WaitGroup`:
  `clk.Run(ctx, p)` (ticking and queuing lines) and `p.Run(ctx)` (the
  printer's own worker, actually writing them). Returns immediately. Both
  `RunConsole` and `RunUI` build on the same `*App` instead of each setting
  up their own clock and printer.
- `(*App).Wait()` — blocks until both of those goroutines have exited.
  Requires `ctx` to already be cancelled (or about to be), otherwise it
  blocks forever; see the shutdown sequence below.
- `(*App).RunConsole(ctx context.Context)` — plain-console mode. Since
  `Start` already has the clock printing in the background, all this does
  is block on `<-ctx.Done()` (Ctrl-C).

## `ui.go` — the `--ui` mode-switching loop

- `RunUI(a *App, autoSwitch <-chan struct{}) error` is all of the `--ui`
  mode-switching logic:
  1. Enters text mode via `textmode.Run(autoSwitch)` (see
     [../textmode/README.md](../textmode/README.md)).
  2. Once that first switch happens (by keypress or `autoSwitch`), loops
     calling `tui.Run(a.Clock)` (see [../tui/README.md](../tui/README.md))
     and `textmode.Run(nil)` alternately, based on which `Signal` each one
     returns (`SwitchToText`/`SwitchToUI` or `Quit`), enabling/disabling
     `a.Printer` to match whichever mode is currently active.

  `RunUI` itself has no notion of *why* or *when* `autoSwitch` fires —
  that's entirely up to the caller.

  Each time it's about to hand the screen to tview, `RunUI` also calls a
  small internal `flushBeforeUI` helper, which calls `a.Printer.Flush`
  with a short (`preUIFlushTimeout`, 200ms) timeout. This is a best-effort
  cleanliness step: without it, a console line still sitting in the
  printer's queue could get written *after* tview has already cleared and
  taken over the screen, since `Println` is asynchronous (see
  [../printer/README.md](../printer/README.md)). It's not
  correctness-critical, so a missed flush just proceeds anyway rather than
  blocking startup.

## How `main.go` drives this

`main.go` parses the `--ui` flag and a `signal.NotifyContext` for Ctrl-C,
calls `app.Start(ctx)` to get the shared `*App`, and only then decides what
to do with it: if `--ui`, it builds the external auto-switch trigger (below)
and calls `app.RunUI(a, autoSwitch)`; otherwise it calls `a.RunConsole(ctx)`.

Either way, once that call returns, `main.go` runs the same shutdown
sequence: `cancel()` the context (telling `Clock.Run` and `Printer.Run` to
stop) and then `a.Wait()` (blocking until the printer has actually drained
its queue and both background goroutines have exited) *before* checking
the error and possibly calling `os.Exit`. This is what makes the printer's
asynchrony safe: without waiting, a line queued just before exit could be
silently dropped when the process terminates.

## External auto-switch trigger

`RunUI` doesn't have a "switch to UI after N seconds" feature built in.
Instead it takes an `autoSwitch <-chan struct{}` parameter and treats it
exactly like a spacebar press: closing it asks for an immediate switch to
UI mode, whenever that happens. `main.go` is the one piece of code that
actually decides *when*: it creates the channel and calls
`time.AfterFunc(3*time.Second, func() { close(autoSwitch) })` before
calling `RunUI`.

This keeps the "switch after 3 seconds on startup" policy out of `app` and
`textmode` entirely — they only know about a generic external signal. In
this app that signal happens to be a timer, but it stands in for any other
real-world trigger (e.g. an event from elsewhere in a larger program) that
might want to force the UI open without the user pressing space.
