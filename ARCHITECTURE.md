# Architecture

A small demo app that prints the current date/time every 2 seconds, with two
front ends: plain console and a `tview`-based TUI that can be toggled live.

This file stays high-level. Each package's detailed design lives in its own
`README.md`, linked from the package list below.

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

`--ui` mode **always starts in text mode**, then auto-switches to the tview
UI on a startup timer (or sooner, on space). After that first transition,
space toggles between the two for as long as the app runs. Quitting from
either mode (`q`/`Q`/Ctrl-C) always leaves the terminal back in normal
console state before the process exits. See
[internal/app/README.md](internal/app/README.md) for how that's wired.

## Core idea: one stored value, one push consumer, one pull consumer

The thing the app actually *does* — refresh "now" every 2 seconds — lives in
`internal/clock`. It *pushes* every update to a `printer.Printer` (which
decides whether that's actually visible), while the UI instead *pulls* the
value on its own schedule. Neither consumer needs the clock to know about
modes. Details and rationale: [internal/clock/README.md](internal/clock/README.md).

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

- [internal/clock](internal/clock/README.md) — owns and refreshes the
  current time, pushing it to a printer and exposing it for the UI to pull.
- [internal/printer](internal/printer/README.md) — the dedicated,
  disableable, asynchronous stdout writer (own background worker + queue,
  with a `Flush` for graceful handoffs/shutdown).
- [internal/textmode](internal/textmode/README.md) — raw-mode, single-key
  console input (space / q / Q / Ctrl-C / external switch signal).
- [internal/tui](internal/tui/README.md) — the tview-based full-screen UI.
- [internal/app](internal/app/README.md) — wires everything together:
  shared app startup, plain console mode, and the `--ui` mode-switching
  loop. Also covers how `main.go` drives it and the external auto-switch
  trigger.
