# Architecture

A small demo app that prints the current date/time every 2 seconds, with two
front ends: plain console and a `tview`-based TUI that can be toggled live.

## Run modes

```
go run .            # plain console mode (default)
go run . --ui        # UI mode: starts in text mode, auto-switches to tview UI after 3s
```

| Mode                | How to enter                              | How to leave                          |
|---------------------|--------------------------------------------|----------------------------------------|
| Console (no `--ui`) | default                                    | Ctrl-C (process signal)                |
| `--ui` text mode    | when `--ui` starts; or pressing **space** while in UI mode | press **space** → UI mode; 3s startup timer → UI mode (first time only); `q`/`Q`/Ctrl-C → quit |
| `--ui` UI mode      | automatically 3s after startup if nothing else happens first; or pressing **space** while in text mode | press **space** → text mode; `q`/`Q`/Ctrl-C → quit |

`--ui` mode **always starts in text mode**. `main.go` starts a 3-second timer
when launching UI mode; if it fires (and the user hasn't already pressed
space), it asks `app.RunUI` to switch to the tview UI — see "External
auto-switch trigger" below. After that first transition, space toggles
between the two for as long as the app runs. Quitting from either mode (via
`q`/`Q`/Ctrl-C) always leaves the terminal back in normal console state
before the process exits, because both the tview screen and the raw
terminal mode are torn down before `Run` returns.

## Core idea: one stored value, one push consumer, one pull consumer

The thing the app actually *does* — refresh "now" every 2 seconds — lives in
`internal/clock`. `Clock.Run` owns the value: on every tick it updates the
stored time and *pushes* it straight to a `printer.Printer` (which decides
whether that's actually visible). The UI does not get pushed to; instead it
*pulls* the value on its own schedule via `Clock.Now()`. Neither consumer
needs the clock to know about modes, and the UI's refresh cadence is fully
decoupled from the clock's tick interval.

```
                 ┌──────────────┐
                 │  clock.Clock  │  every tickInterval: now = time.Now(); p.Println(now)
                 │  (stores now) │
                 └──────┬────────┘
            pushed via  │              polled via
            Run(ctx, p)  │              Now()
                         ▼                   ▲
              printer.Printer.Println   tui's own ticker (refreshInterval)
              (no-op when Disabled)      reads clk.Now(), redraws TextView
```

## Packages

- **`internal/clock`** — `Clock.Run(ctx, p)` is the main loop: every
  `tickInterval` it stores `time.Now()` and pushes it to the given
  `printer.Printer`. `Clock.Now()` lets any other consumer (the UI) pull the
  latest stored value on its own schedule. The clock has no notion of
  "modes" and no subscriber list — it just stores a value and prints it.

- **`internal/printer`** — `Printer` is the dedicated, disableable print
  class requested: `Println` only writes to stdout when `Enable()`d;
  `Disable()` makes it a no-op. The clock-feeding goroutine always calls
  `Printer.Println`, whether or not output is actually visible — the
  printer, not the caller, decides.

- **`internal/textmode`** — implements the plain-console *input* side of
  `--ui` mode: puts stdin into raw mode (via `golang.org/x/term`) so single
  keypresses (space, `q`/`Q`, Ctrl-C) can be read without waiting for
  Enter, while stdout keeps behaving like a normal scrolling terminal
  (scrollback works, since raw mode only affects input echo/buffering, not
  the scrollback buffer). Restores the terminal before returning.

  `Run(switchSignal <-chan struct{})` waits for either a keypress or
  `switchSignal` to fire — both space and a closed `switchSignal` return
  `SwitchToUI`; `q`/`Q`/Ctrl-C return `Quit`. Internally it polls stdin
  with `golang.org/x/sys/unix.Poll` on a short fixed interval rather than
  doing one blocking `Read`, so it can also check `switchSignal` regularly
  and so that, whichever fires, there's never a goroutine left blocked on
  stdin afterwards (which would otherwise race a later call to `Run`).
  Pass `nil` for `switchSignal` to only react to keypresses.

- **`internal/tui`** — the tview UI: a single bordered `TextView`, redrawn
  by its own `refreshInterval` ticker (500ms) that calls `clk.Now()` and
  pushes the result into the view via `QueueUpdateDraw`. This polling loop
  is local to `tui.Run` and stops as soon as it returns. `SetInputCapture`
  intercepts space and quit keys and calls `Application.Stop()`, which
  tears down the tview/tcell screen (restoring the terminal) before `Run`
  returns.

- **`internal/app`** is split into two files so application logic and UI
  logic don't mix, and so the clock/printer are only ever created once:
  - **`app.go`** — `App` holds the shared `Clock` and `Printer`.
    `Start(ctx)` creates them and starts `clk.Run(ctx, p)` in the
    background, returning immediately; both `RunConsole` and `RunUI` build
    on the same `*App` instead of each setting up their own clock and
    printer. `(*App).RunConsole(ctx)` is plain-console mode: since `Start`
    already has the clock printing in the background, all it has to do is
    block on `<-ctx.Done()` (Ctrl-C).
  - **`ui.go`** — `RunUI(a *App, autoSwitch <-chan struct{})` is all of the
    `--ui` mode-switching logic: it enters text mode via
    `textmode.Run(autoSwitch)`, and once that first switch happens (by
    keypress or `autoSwitch`), loops calling `tui.Run(a.Clock)` and
    `textmode.Run(nil)` alternately based on which `Signal` each one
    returns (`SwitchToText`/`SwitchToUI` or `Quit`), enabling/disabling
    `a.Printer` to match whichever mode is currently active. `RunUI` itself
    has no notion of *why* or *when* `autoSwitch` fires — that's entirely
    up to the caller.

- **`main.go`** — parses the `--ui` flag and a `signal.NotifyContext` for
  Ctrl-C, calls `app.Start(ctx)` to get the shared `*App`, and only then
  decides what to do with it: if `--ui`, it builds the "external
  auto-switch trigger" channel (see below) and calls `app.RunUI(a,
  autoSwitch)`; otherwise it calls `a.RunConsole(ctx)`.

## External auto-switch trigger

`app.RunUI` doesn't have a "switch to UI after N seconds" feature built in.
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

## Why push for the printer but pull for the UI?

The printer is meant to mirror exactly what the clock logic produces, at
the clock's own cadence, so the clock pushes to it directly inside `Run`.
The UI, on the other hand, wants to redraw at whatever cadence makes the
screen feel responsive (and only while it's actually on screen), which has
nothing to do with how often the underlying value changes — so it pulls via
`Now()` on its own ticker instead of being pushed to. This keeps `Clock`
simple (no subscriber bookkeeping) while letting each consumer pick the
cadence that suits it.
