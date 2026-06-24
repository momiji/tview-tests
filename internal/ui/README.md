# internal/ui

Everything that presents the clock's value to a human: plain console
output (`console.go`), the `--ui` mode-switching loop (`ui.go`), and the
two presentation packages it switches between,
[textmode](textmode/README.md) and [tui](tui/README.md). See
[../../ARCHITECTURE.md](../../ARCHITECTURE.md) for how this fits into the
rest of the app.

## `console.go` — plain console mode

`RunConsole(ctx context.Context)` is plain-console mode. The clock and
printer are started and run entirely by `internal/app`
([README](../app/README.md)) in the background, so this just blocks on
`<-ctx.Done()` (Ctrl-C).

## `ui.go` — the `--ui` mode-switching loop

`RunUI(clk *clock.Clock, p *printer.Printer, autoSwitch <-chan struct{}) error`
is all of the `--ui` mode-switching logic:

1. Enters text mode via `textmode.Run(autoSwitch)` (see
   [textmode/README.md](textmode/README.md)).
2. Once that first switch happens (by keypress or `autoSwitch`), loops
   calling `tui.Run(clk)` (see [tui/README.md](tui/README.md)) and
   `textmode.Run(nil)` alternately, based on which `Signal` each one
   returns (`SwitchToText`/`SwitchToUI` or `Quit`), enabling/disabling `p`
   to match whichever mode is currently active.

`RunUI` itself has no notion of *why* or *when* `autoSwitch` fires — that's
entirely up to the caller (see "External auto-switch trigger" below).

Each time it's about to hand the screen to tview, `RunUI` also calls a
small internal `flushBeforeUI` helper, which calls `p.Flush` with a short
(`preUIFlushTimeout`, 200ms) timeout. This is a best-effort cleanliness
step: without it, a console line still sitting in the printer's queue
could get written *after* tview has already cleared and taken over the
screen, since `Println` is asynchronous (see
[../service/printer/README.md](../service/printer/README.md)). It's not
correctness-critical, so a missed flush just proceeds anyway rather than
blocking startup.

## External auto-switch trigger

`RunUI` doesn't have a "switch to UI after N seconds" feature built in.
Instead it takes an `autoSwitch <-chan struct{}` parameter and treats it
exactly like a spacebar press: closing it asks for an immediate switch to
UI mode, whenever that happens. `cmd/tview-tests/main.go` is the one piece
of code that actually decides *when*: it creates the channel and calls
`time.AfterFunc(3*time.Second, func() { close(autoSwitch) })` before
calling `app.Run` (which forwards it into `RunUI`).

This keeps the "switch after 3 seconds on startup" policy out of `ui` and
`textmode` entirely — they only know about a generic external signal. In
this app that signal happens to be a timer, but it stands in for any other
real-world trigger (e.g. an event from elsewhere in a larger program) that
might want to force the UI open without the user pressing space.
