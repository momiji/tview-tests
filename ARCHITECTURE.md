# Architecture

A small demo app that prints the current date/time every 2 seconds, with two
front ends: plain console and a `tview`-based TUI that can be toggled live.

This file stays high-level. Each package's detailed design lives in its own
`README.md`, linked from the package list below.

## Layout

```
cmd/tview-tests/   the binary's entrypoint (flag parsing, signal context)
internal/
  app/             orchestrator: creates the shared components, starts them,
                   runs whichever mode main asked for, waits for shutdown
  ui/              everything that presents the clock to a human
    textmode/      raw-mode single-key console input
    tui/           the tview full-screen UI
  service/         standalone background components, not UI-specific
    clock/         the core "current time" value
    printer/       the toggleable, asynchronous stdout writer
```

`service/` is where future non-UI components (config, a socket/net server,
etc.) would go; `ui/` is where future *presentation* modes would go. Both
groups exist as of this writing with exactly the components above — the
split is there so the boundary is already in place before anything new
needs to pick a side. Components under `service/` are the ones expected to
eventually need real infrastructure (e.g. a docker-compose stack) for
integration testing; `clock` and `printer` themselves don't need any.

## Run modes

```
go run ./cmd/tview-tests            # plain console mode (default)
go run ./cmd/tview-tests --ui        # UI mode: starts in text mode, auto-switches to tview UI after 3s
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
[internal/app/README.md](internal/app/README.md) and
[internal/ui/README.md](internal/ui/README.md) for how that's wired.

## Core idea: one stored value, one push consumer, one pull consumer

The thing the app actually *does* — refresh "now" every 2 seconds — lives in
`internal/service/clock`. It *pushes* every update to a `printer.Printer`
(which decides whether that's actually visible), while the UI instead
*pulls* the value on its own schedule. Neither consumer needs the clock to
know about modes. Details and rationale:
[internal/service/clock/README.md](internal/service/clock/README.md).

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

- [internal/app](internal/app/README.md) — the orchestrator: creates and
  starts the shared `Clock`/`Printer`, runs the chosen mode, waits for
  clean shutdown.
- [internal/ui](internal/ui/README.md) — plain console mode, the `--ui`
  mode-switching loop, and:
  - [internal/ui/textmode](internal/ui/textmode/README.md) — raw-mode,
    single-key console input (space / q / Q / Ctrl-C / external switch
    signal). The only package with OS-specific implementations
    (Linux/macOS vs. Windows).
  - [internal/ui/tui](internal/ui/tui/README.md) — the tview-based
    full-screen UI.
- [internal/service/clock](internal/service/clock/README.md) — owns and
  refreshes the current time, pushing it to a printer and exposing it for
  the UI to pull.
- [internal/service/printer](internal/service/printer/README.md) — the
  dedicated, disableable, asynchronous stdout writer (own background
  worker + queue, with a `Flush` for graceful handoffs/shutdown), plus
  formatted/request-trace logging methods built on top of it (migrated
  from kpx).
