# internal/ui/tui

The tview-based full-screen UI. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits
into the rest of the app.

## API

- `Run(clk *clock.Clock) (Signal, error)` — renders a single bordered
  `TextView` until the user presses space (`SwitchToText`) or `q`/`Q`/
  Ctrl-C (`Quit`).

## Implementation notes

- The view is redrawn by its own `refreshInterval` ticker (500ms) that
  calls `clk.Now()` (see
  [../../service/clock/README.md](../../service/clock/README.md)) and
  pushes the result into the view via `QueueUpdateDraw`. This polling loop
  is local to `Run` and stops as soon as it returns — `tui` pulls from the
  clock rather than being pushed to, on whatever cadence makes the screen
  feel responsive, independent of the clock's own tick interval.
- `SetInputCapture` intercepts space and quit keys and calls
  `Application.Stop()`, which tears down the tview/tcell screen
  (restoring the terminal) before `Run` returns — so the caller can safely
  fall back to plain console mode (see
  [../textmode/README.md](../textmode/README.md)) immediately after.
