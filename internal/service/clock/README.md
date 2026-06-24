# internal/service/clock

Owns the application's core value: the current time, refreshed on a fixed
interval. See [../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how
this fits into the rest of the app.

## API

- `New(interval time.Duration) *Clock` — creates a clock holding the
  current time, ticking every `interval` once `Run` is called.
- `(*Clock).Run(ctx context.Context, p *printer.Printer)` — the main loop:
  every tick it stores `time.Now()` and pushes it to `p.Println`. Returns
  when `ctx` is cancelled.
- `(*Clock).Now() time.Time` — returns the most recently stored time, for
  pulling instead of being pushed to.
- `Format` — the `time.RFC1123` layout used to render the time everywhere
  in the app.

`Clock` has no notion of "modes" and no subscriber list — it just stores a
value, prints it, and lets anyone else read it.

## Why push for the printer but pull for the UI?

The printer is meant to mirror exactly what the clock logic produces, at
the clock's own cadence, so the clock pushes to it directly inside `Run`.
The UI (see [../../ui/tui/README.md](../../ui/tui/README.md)), on the
other hand, wants to redraw at whatever cadence makes the screen feel
responsive (and only while it's actually on screen), which has nothing to
do with how often the underlying value changes — so it pulls via `Now()`
on its own ticker instead of being pushed to. This keeps `Clock` simple
(no subscriber bookkeeping) while letting each consumer pick the cadence
that suits it.
