# internal/proxy/upstream

Selects which upstream proxy (and which of its hosts) to use for a request,
with failover and high availability. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits into
the rest of the app.

## API

- `NewSelector(conf *config.ProxyConf, p *printer.Printer) *Selector`.
- `(*Selector).FindFirstProxy(rule, proxies) (*config.Proxy, string)` —
  given the candidate proxies for a rule (already ordered by the router),
  returns the first reachable proxy and the `host:port` to dial. `direct` and
  `none` proxies are returned immediately without probing.

## How selection works

Candidates are ranked by "last reachable" time (most recent first), then
probed in order; a proxy with several comma-separated hosts has its hosts
ranked and probed the same way. The first host that accepts a TCP connection
wins, and its proxy/host are recorded as reachable so they are preferred next
time. This "last reachable" map is the mutable HA state, kept here rather
than on the immutable `config.ProxyConf` (MIGRATION.md §2).

## Notes on the port

- The `lastProxies`/`lastMutex` state moved out of the old config into this
  selector. Debug logging goes through the injected `*printer.Printer`, gated
  on `conf.Debug` (the old global `debug`).

## Limitations

- **In-place sort of the caller's slice.** `FindFirstProxy` sorts the passed
  `proxies` slice in place (`append(proxies[:0], proxies...)`), which aliases
  and reorders the caller's backing array — including the router's cached
  slice. Faithful to the original but a shared-state hazard; revisit (copy
  first) when hardening concurrency.
- **`host:port` via `net.JoinHostPort`.** The original used
  `fmt.Sprintf("%s:%d", ...)`; switched to `net.JoinHostPort` to satisfy
  `go vet` and bracket IPv6 correctly (a minor improvement).
- **`proxyShortName` is a local copy** (also used by the processor); to be
  shared once the processor is migrated.
