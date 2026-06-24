# Architecture

A small demo app that prints the current date/time every 2 seconds, with two
front ends: plain console and a `tview`-based TUI that can be toggled live.

## Run modes

```
go run .            # plain console mode (default)
go run . --ui        # UI mode: starts in text mode, auto-switches to tview UI
```

| Mode                | How to enter                              | How to leave                          |
|---------------------|--------------------------------------------|----------------------------------------|
| Console (no `--ui`) | default                                    | Ctrl-C (process signal)                |
| `--ui` text mode    | automatic, briefly, when `--ui` starts; or pressing **space** while in UI mode | press **space** → UI mode; `q`/`Q`/Ctrl-C → quit |
| `--ui` UI mode      | automatic right after startup; or pressing **space** while in text mode       | press **space** → text mode; `q`/`Q`/Ctrl-C → quit |

`--ui` mode **always starts in text mode and immediately auto-switches to UI
mode** — no keypress needed for that first transition. After that, space
toggles between the two for as long as the app runs. Quitting from either
mode (via `q`/`Q`/Ctrl-C) always leaves the terminal back in normal console
state before the process exits, because both the tview screen and the raw
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

- **`internal/tui`** — the tview UI: a single bordered `TextView`, redrawn
  by its own `refreshInterval` ticker (500ms) that calls `clk.Now()` and
  pushes the result into the view via `QueueUpdateDraw`. This polling loop
  is local to `tui.Run` and stops as soon as it returns. `SetInputCapture`
  intercepts space and quit keys and calls `Application.Stop()`, which
  tears down the tview/tcell screen (restoring the terminal) before `Run`
  returns.

- **`internal/app`** — orchestrates the two top-level entry points:
  - `RunConsole(ctx)`: the original plain-console behavior — `clk.Run(ctx,
    p)` directly, printing forever until `ctx` is cancelled (Ctrl-C).
  - `RunUI(ctx)`: starts `clk.Run(ctx, p)` in the background, disables the
    printer, and loops calling `tui.Run` and `textmode.Run` alternately
    based on which `Signal` each one returns (`SwitchToText`/`SwitchToUI`
    or `Quit`), enabling/disabling the printer to match whichever mode is
    currently active.

- **`main.go`** — parses the `--ui` flag and a `signal.NotifyContext` for
  Ctrl-C, then dispatches to `app.RunConsole` or `app.RunUI`.

## Why push for the printer but pull for the UI?

The printer is meant to mirror exactly what the clock logic produces, at
the clock's own cadence, so the clock pushes to it directly inside `Run`.
The UI, on the other hand, wants to redraw at whatever cadence makes the
screen feel responsive (and only while it's actually on screen), which has
nothing to do with how often the underlying value changes — so it pulls via
`Now()` on its own ticker instead of being pushed to. This keeps `Clock`
simple (no subscriber bookkeeping) while letting each consumer pick the
cadence that suits it.
