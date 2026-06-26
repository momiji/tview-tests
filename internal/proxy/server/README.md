# internal/proxy/server

Runs the inbound listeners — the HTTP proxy and the SOCKS5 server — enforces
the ACL, and hands each accepted connection to a `processor`. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits into
the rest of the app.

## API

- `New(runtime *processor.Runtime, conf *config.ProxyConf, p *printer.Printer)
  *Server`.
- `(*Server).Run(ctx context.Context) error` — starts the HTTP listener
  (`conf.Port`) and the SOCKS5 server (`conf.SocksPort`) and blocks until
  `ctx` is cancelled or a listener fails. Cancelling `ctx` closes the
  listeners and returns.
- `TCPHandle` / `UDPHandle` — the `socks5` server callbacks (TCP runs a
  processor; UDP is not implemented).

## Notes on the port

- Shutdown is by **context cancellation** instead of the old
  `ManualResetEvent`/`os.Exit`: cancelling the context closes the HTTP
  listener (unblocking `Accept`) and stops the SOCKS server. The same context
  is shared with the `processor.Runtime`, so in-flight processors see
  `stopped()` too.
- The connection-pool vacuum goroutine is gone (pool dropped, §5).
- Each accepted connection is checked against the ACL, TCP-tuned
  (`transport.ConfigureConn`), then handled in its own goroutine.

## Limitations

- **No single-port demux yet.** HTTP and SOCKS use separate ports; the
  optional one-port HTTP/SOCKS demux and the local `proxy.pac` web handler
  unification (MIGRATION.md §3.5) are not implemented — the `proxy.pac` is
  served inline by the HTTP processor.
- **Runtime wiring lives elsewhere.** Building the `Runtime` (loading the
  config, kerberos store, router, selector, cert manager) is done by the app
  layer (migration step 13); this package only runs the listeners.
