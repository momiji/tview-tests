# internal/proxy/processor

Handles a single proxied connection end to end: read the request, match it
to a rule and upstream proxy, authenticate, and move the bytes. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits into
the rest of the app.

## Structure (by axes, MIGRATION.md §3)

- **`Runtime`** (`runtime.go`) — the shared services (router, upstream
  selector, kerberos store, cert manager, printer) and the shutdown context.
  Replaces the old global orchestrator state; shutdown is by context
  cancellation, not `os.Exit`. Provides the Kerberos token generation.
- **`Process`** (`process.go`) — per-connection state and `processChannel`,
  the orchestration: read headers, serve the local `proxy.pac`, match, select
  upstream, authenticate, dial, forward, then tunnel / mitm / keep-alive.
- **Connectors (axis A)** — `connectDirect` (`connector.go`), `connectForward`
  (`forward.go`), `connectSocks` (`socks.go`): establish the upstream
  connection per proxy type.
- **Authenticator (axis B)** — `authenticator.go`: `computeAuthPerConf` /
  `computeAuthPerUser` build the upstream authorization (kerberos / basic /
  socks, per-conf and per-user).
- **Transport bricks (axis C)** — `transport_http.go` (forward
  request/response), `transport_tunnel.go` (duplex pipe),
  `transport_mitm.go` (TLS-terminated request/response loop).
- **`TrafficRow`** (`traffic.go`) — minimal byte-counter implementing
  `transport.TrafficMeter`.

## Notes on the port

- The connection **pool** and `TimedConn` are gone (MIGRATION.md §5): each
  request dials a fresh upstream; client-side keep-alive is preserved by the
  `ProcessHttp` loop. The only timeout kept is a one-shot read deadline
  (`ConnectTimeout`) around the request-header read.
- The global `debug`/`trace`/`logger` are gone: verbosity comes from the
  resolved config (`conf.Debug`/`conf.Trace` and the per-rule/proxy effective
  values), logging through the injected `*printer.Printer`.

## Limitations

- **`forward` and `socks` connectors are stubs** (steps 9b/9d). Today only
  `none` and `direct` (http / tunnel / mitm) are wired; kerberos/basic/
  anonymous/socks upstreams return "not yet migrated".
- **SOCKS server entry not ported** (`processSocks` / `TCPHandle`, step 9d).
- **No traffic registry / UI table.** `TrafficRow` counts bytes but is not yet
  registered in a UI table (MIGRATION.md step 14); `closeChannels` does not
  deregister it.
- **MITM needs a wired cert manager.** `transport_mitm` calls
  `Runtime.certs`; the CA/`cert.Manager` orchestration (old `genCerts`) is
  wired in a later step, so MITM is only functional once that lands.
- **Diagnostic response headers renamed** to the neutral `x-proxy-*` prefix
  (the old product-named headers cannot appear in shipped code); behavior is
  otherwise unchanged (see IDEAS.md).
