# internal/service/printer

The dedicated, disableable, **asynchronous** stdout writer. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits
into the rest of the app.

## API

- `New() *Printer` — returns a `Printer` that starts enabled. Queues lines
  in memory; nothing is written to stdout until `Run` is also started.
- `(*Printer).Run(ctx context.Context)` — the background worker: writes
  queued lines to stdout, in order, until `ctx` is cancelled, then drains
  whatever is still queued before returning. Mirrors the
  `Clock.Run(ctx, p)` pattern in [../clock/README.md](../clock/README.md) —
  this package owns a goroutine the same way the clock does.
- `(*Printer).Enable()` / `(*Printer).Disable()` — toggle output. `Disable`
  takes effect immediately: it stops `Println` from queuing anything new,
  *and* makes the worker discard (rather than write) any lines that were
  already queued from before `Disable` was called. Without that second
  part, a backlog queued while enabled would still leak out after
  disabling, just delayed — which would defeat the point of disabling.
  `write()` checks `enabled` under the same mutex `Disable` takes, and
  holds that mutex for the actual `fmt.Print` too — not just the read —
  so once `Disable()` returns, no line can still be mid-print with a
  stale "enabled" value (an earlier version checked-then-unlocked before
  printing, which left exactly that race open).
- `(*Printer).Println(a ...any)` — formats a line and enqueues it for the
  worker to write; a no-op when disabled. Never blocks on I/O: if the
  internal queue (`queueSize`, currently 10000 — sized for bursts of
  per-request trace logging, not just the clock's one-line-every-2s
  ticks; matches kpx's old `DefaultQueueSize`, the equivalent knob in
  go-logging's async logger) is full, the line is dropped rather than
  stalling the caller.
- `(*Printer).Flush(ctx context.Context) error` — blocks until every line
  queued before the call has been processed by the worker (written, or
  discarded if disabled in the meantime), or returns `ctx.Err()` if `ctx`
  is cancelled first. Implemented by queuing a marker job after them and
  waiting for the worker to reach it — relies on `Run` having been
  started.

## Logging methods (migrated from kpx)

Two more files in this package add formatted logging on top of `Printer`,
ported from kpx's `log.go`. Both go through `Println` like everything
else, so log lines share the same async/toggleable behavior as the
clock's output — no separate logging library or lifecycle (kpx's
`logInit`/`logDestroy`/`logWriter`/`logFlush`) is needed here: `New`/`Run`
and `Enable`/`Disable`/`Flush` already cover that.

- `logger.go` — general-purpose formatted logging: `(*Printer).Printf`,
  `Infof`, `Errorf`. Each prefixes a timestamp and queues the result via
  `Println`. kpx's `logFatal` was not migrated.
- `request.go` — request/trace-specific logging, kept separate from the
  general-purpose methods above because it's only meaningful while
  processing a request: `ReqLogInfo`/`NewReqLogInfo` tag a request with an
  id and stage name, `(*Printer).ReqInfof` logs a line tagged with one,
  and `(*Printer).ReqHeaderf` logs an HTTP header while redacting
  `Proxy-Authorization` values down to a short prefix.

The clock (see [../clock/README.md](../clock/README.md)) always calls
`Printer.Println` on every tick, whether or not output is actually
visible — the printer, not the caller, decides. This is what lets
[../../ui/README.md](../../ui/README.md) silence plain-console output
while the tview UI is showing, and re-enable it when switching back,
without the clock's own logic ever branching on mode.

## Why asynchronous?

`Println` is called from the clock's tick loop; if it wrote to stdout
synchronously, a slow or blocked terminal would stall the clock itself.
Queuing the line and handing the actual `fmt.Print` off to `Run`'s own
goroutine decouples "the value changed" from "the value got written
somewhere," the same way `Clock.Now()` decouples "the value changed" from
"the UI redrew." See [../../app/README.md](../../app/README.md) for how
`Start` runs `Printer.Run` and `Clock.Run` side by side, and how shutdown
waits for both to fully drain before the process exits.
