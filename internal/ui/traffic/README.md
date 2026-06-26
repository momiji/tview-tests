# internal/ui/traffic

The per-connection traffic model shown by the UI. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits into the
rest of the app.

## API

- `TrafficRow` — one connection's counters: id, URL, sent/received byte-rate
  counters, and last-activity timestamps. It implements
  `transport.TrafficMeter` (`AddReceived`/`AddSent`), so the proxy updates it
  directly while forwarding.
- `TrafficTable` — a thread-safe slice of rows (`Add`, `RowsCopy`, `Remove`,
  `RemoveDead`). `Remove` marks a row; `RemoveDead` drops rows marked more
  than 30s ago.
- `Sink` — adapts a `TrafficTable` to the processor's `TrafficSink` port:
  `New` creates and registers a metered row per request, `Remove` retires it.
  This keeps the processor independent of the UI.

## Limitations

- **Keep-alive rows accumulate.** A new row is created per request; on a
  kept-alive connection only the last is `Remove`d, so intermediate rows are
  never marked and `RemoveDead` never drops them (inherited; see IDEAS.md).
