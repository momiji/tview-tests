# internal/proxy/router

Decides, at runtime, which rule and which upstream proxies a request maps
to, and generates the local `proxy.pac` served to clients. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits into
the rest of the app.

## API

- `NewRouter(conf *config.ProxyConf, p *printer.Printer) (*Router, error)` —
  downloads the PAC scripts of the used `pac` proxies (filling each one's
  `PacJs`/`PacRuntime` on the resolved config), then generates `proxy.pac`.
  This is the only network step.
- `(*Router).MatchHttp(url, hostPort)` / `MatchSocks(hostPort)` — return the
  matching rule and the ordered upstream proxies for a request, resolving
  PAC rules and caching the result per host.
- `(*Router).Pac()` — the generated `proxy.pac` string.
- `(*Router).NeedFastReload()` — true when a PAC download failed and a quick
  reload should be scheduled.

## How matching works

`match` scans the rules in order. A rule's host `Regex` is tested against the
full url, the `host:port`, or the host only, depending on its shape (and the
`ExperimentalHostsCache` switch). On a hit, `resolve` returns the rule's
proxies; for a `pac` rule it runs the PAC script (`resolvePac`) and maps the
result to an existing pac proxy or a freshly minted temporary one. A PAC
`DIRECT` yields a "continue" sentinel so scanning falls through to a later
rule, or to a final `direct` proxy. Results are memoized in a per-host cache.

## Notes on the port

- The mutable host cache lives here, not on the (immutable) resolved config,
  per [MIGRATION.md](../../../MIGRATION.md) §2. The PAC download moved out of
  `config.build` (no network there) into `NewRouter`.
- PAC evaluation goes through `service/pac`; logging through the injected
  `*printer.Printer`.

## Limitations

- **`PacRuntime`/`PacJs` are written after build.** `NewRouter` mutates the
  resolved config to attach the downloaded PAC runtimes. This is a one-time
  setup mutation, but it means a `ProxyConf` is only fully usable once a
  router has been built for it.
- **`splitHostPort` is a local copy** (also in `config`, `kerberos`,
  `message`); to be folded into a shared util at the CLI step.
- **No `proxy.pac` HTTP handler yet.** This package produces the `proxy.pac`
  string; serving it on the local web endpoint is the `server`'s job (a
  later step, see [MIGRATION.md](../../../MIGRATION.md) §3.5).
