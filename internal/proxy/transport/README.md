# internal/proxy/transport

Low-level connection helpers shared by the proxy. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits into
the rest of the app.

## API

- `ConfigureConn(conn)` — disables Nagle's algorithm on the underlying TCP
  connection (unwrapping a `*tls.Conn` if needed) to reduce TIME_WAIT
  connections.
- `TrafficConn` — a `net.Conn` wrapper that reports read/written byte counts
  to an optional `TrafficMeter`. `NewTrafficConn(conn)` starts without a
  meter; `SetMeter(m)` attaches one (nil disables accounting).
- `TrafficMeter` — the port (`AddReceived(n)`, `AddSent(n)`) through which a
  UI's per-connection traffic row is updated. Defining it here keeps this
  package independent of the UI.
- `NewChunkedReader(r)` / `NewChunkedWriter(w)` — an instrumented copy of the
  standard library's HTTP "chunked" codec that preserves the raw chunk lines
  instead of converting them, so headers can be forwarded verbatim.

## Notes on the port

- `TrafficConn` now uses a pointer receiver and an injected `TrafficMeter`
  interface. Previously it held a concrete `*ui.TrafficRow` and used value
  receivers, which silently lost its byte-batching state (a latent bug); the
  byte counts are now reported directly on each read/write.

## Limitations

- **`TrafficMeter` has no implementation yet.** The concrete per-connection
  traffic row lives in the UI, which is reconciled in a later migration step
  (see [../../../MIGRATION.md](../../../MIGRATION.md) step 14). Until then,
  `TrafficConn` runs with a nil meter (no accounting) and the UI row must be
  adapted to implement `TrafficMeter`.
- **`TimedConn` and `CloseAwareConn` were dropped on purpose**
  (see [MIGRATION.md](../../../MIGRATION.md) §5): the sliding/idle/close
  timeouts and the pool-only close-detection wrapper are gone. Bounding the
  request-header read is done by the processor with a one-shot
  `conn.SetReadDeadline(now + connectTimeout)`, without a wrapper.
