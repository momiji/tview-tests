# Architecture

A Kerberos-authenticating HTTP/1.1 and SOCKS5 proxy. It exposes a local
anonymous proxy that forwards requests to upstream proxies/servers,
automatically injecting the required credentials (Kerberos/Negotiate, Basic,
or SOCKS auth), and also serves a generated `proxy.pac`. Which upstream to use
per request is decided by configurable rules and PAC scripts.

This file stays high-level. Each package's detailed design lives in its own
`README.md`, linked from the package list below.

## Layout

```
cmd/kpx/           the binary's entry point (app metadata, calls app.Main)
internal/
  app/             wiring: parse CLI, load config, build services, run
  cli/             flags -> config.CmdArgs + action; usage/help text
  config/          three-layer config model + load/check/build + watch
  proxy/           the proxy engine
    server/        inbound listeners (HTTP + SOCKS5), ACL, accept loop
    processor/     per-connection handling, split by axes (connector /
                   authenticator / transport brick) + the Runtime
    router/        runtime matching (rule + PAC) and proxy.pac generation
    upstream/      upstream proxy/host selection with failover (HA)
    message/       raw HTTP message read/parse/write
    transport/     TCP tuning, byte-counting conn, chunked codec
  service/         standalone, UI-agnostic components
    cert/          self-signed CA + per-host certs (MITM)
    kerberos/      SPNEGO tokens (password cache + native OS SSO)
    secret/        config password encryption
    pac/           PAC script evaluation (goja)
    printer/       the toggleable, asynchronous stdout writer
  ui/              the optional tview traffic table (--ui)
    traffic/       the per-connection traffic model (UI-agnostic)
  update/          optional self-update
```

The domain (`config`, `proxy/*`, `service/*`) never depends on the UI: it
receives its collaborators (a `*printer.Printer`, a `cert.Manager`, a
`kerberos.Store`, a `TrafficSink`) by injection. The resolved configuration
is immutable after build and published atomically, so a hot-reload swaps a
whole new snapshot rather than mutating the live one.

## Run modes

```
kpx                          # use kpx.yaml / kpx.json in the working dir
kpx -c config.yaml           # use an explicit config file
kpx -u user@domain proxy:8080   # single upstream proxy, no config file
kpx -e                       # encrypt a password (prints encrypted: ...)
kpx --ui                     # run with the live tview traffic table
```

| Form                | What it does                                        |
|---------------------|-----------------------------------------------------|
| config file         | full rules/proxies/credentials; hot-reloaded on change |
| single proxy arg    | one upstream (kerberos if `-u`, else anonymous, or direct if port 0), auto-exits after `--timeout` |
| `-e`                | encrypt a password with the key file, then exit     |
| `--ui`              | render the traffic table; `q`/`Q`/Ctrl-C quits      |

Shutdown is by context cancellation: a signal (SIGINT/SIGTERM), the auto-exit
timeout, or a self-update restart all cancel the runtime context, which stops
the listeners and the background tasks.

## Core idea: listen → match → select → authenticate → forward

Each accepted connection is handled by one `processor.Process` (one goroutine,
plus one extra for the duplex pipe). The flow:

```
  client ──▶ server (HTTP/SOCKS listener, ACL)
                │  one Process per connection
                ▼
        read request (message)
                │
                ▼
        router.Match ──▶ rule + candidate upstream proxies   (PAC resolved here)
                │
                ▼
        upstream.FindFirstProxy ──▶ first reachable proxy/host (failover)
                │
                ▼
        connector (direct / http-forward / socks)   axis A
          + authenticator (kerberos / basic / socks) axis B
                │
                ▼
        transport brick (http / tunnel / mitm)       axis C  ──▶ upstream
```

The connection pool and per-connection idle timeouts of the original were
dropped: each request dials fresh and client-side keep-alive is preserved by
the processor loop. Byte counts flow to a `TrafficSink` (a no-op unless the
`--ui` traffic table is attached). See
[internal/proxy/processor/README.md](internal/proxy/processor/README.md).

## Packages

- `cmd/kpx` — the binary entry point: defines the application metadata
  (overridable via `-ldflags`) and calls `app.Main`.
- [internal/app](internal/app/README.md) — wires everything and runs the
  proxy: parse CLI, load config, build services (kerberos/cert/router/
  upstream), start the listeners, and drive hot-reload and self-update.
- [internal/cli](internal/cli/README.md) — parses flags into `config.CmdArgs`
  and an action (run / help / version / encrypt); owns the usage/help text.
- [internal/ui](internal/ui/README.md) — the optional `--ui` tview traffic
  table, with:
  - [internal/ui/traffic](internal/ui/traffic/README.md) — the
    per-connection traffic model (byte-rate rows + table) that the proxy
    feeds and the table renders; UI-agnostic.
- [internal/service/printer](internal/service/printer/README.md) — the
  dedicated, disableable, asynchronous stdout writer (own background
  worker + queue, with a `Flush` for graceful handoffs/shutdown), plus
  formatted/request-trace logging methods built on top of it.
- [internal/service/pac](internal/service/pac/README.md) — evaluates PAC
  (Proxy Auto-Configuration) scripts via goja to pick a proxy for a given
  URL/host.
- [internal/service/secret](internal/service/secret/README.md) — encrypts
  and decrypts configuration passwords with a locally stored key file
  (AES-GCM).
- [internal/service/cert](internal/service/cert/README.md) — the X.509
  building blocks for MITM: a self-signed CA and a cache of per-host leaf
  certificates minted on demand.
- [internal/service/kerberos](internal/service/kerberos/README.md) —
  authenticates upstream proxies with SPNEGO/Negotiate tokens: a cache of
  password-based clients plus the native OS single-sign-on path (ccache on
  Linux, SSPI on Windows).
- [internal/config](internal/config/README.md) — turns command-line
  arguments and a YAML/JSON file into a resolved proxy configuration
  (three layers: `CmdArgs` / `FileConfig` / `ProxyConf`, with a tri-state
  cascade for the verbose/debug/trace/mitm switches). No network, no
  routing.
- [internal/proxy/transport](internal/proxy/transport/README.md) — low-level
  connection helpers: TCP tuning (`ConfigureConn`), a byte-counting
  `TrafficConn` (metered through a UI-agnostic `TrafficMeter` port), and an
  instrumented HTTP "chunked" reader/writer.
- [internal/proxy/message](internal/proxy/message/README.md) — reads, parses
  and writes the raw HTTP messages (`ProxyRequest` / `RequestHeader`),
  keeping header lines verbatim for faithful forwarding.
- [internal/proxy/router](internal/proxy/router/README.md) — runtime
  matching: maps a request to a rule and its upstream proxies (resolving PAC
  scripts, caching per host), and generates the local `proxy.pac`.
- [internal/proxy/upstream](internal/proxy/upstream/README.md) — selects the
  reachable upstream proxy/host for a request (failover + HA), remembering
  the last reachable one outside the immutable config.
- [internal/proxy/processor](internal/proxy/processor/README.md) — handles a
  single connection: match, authenticate, dial and forward, split by axes
  (connector / authenticator / transport brick). Shutdown via context, no
  pool.
- [internal/proxy/server](internal/proxy/server/README.md) — the inbound
  listeners (HTTP proxy + SOCKS5), ACL enforcement and accept loop; hands
  each connection to a processor. Shuts down on context cancellation.
- [internal/update](internal/update/README.md) — optional self-update: checks
  the releases API and installs a newer binary in place, signaling a restart
  (no `os.Exit`).
