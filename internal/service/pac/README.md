# internal/service/pac

Evaluates PAC (Proxy Auto-Configuration) scripts to decide, for a given
URL/host, what proxy (if any) to use. Migrated from kpx's `pac.go`. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits
into the rest of the app.

## API

- `NewPac(pacJs string, p *printer.Printer, opts *PacOptions) (*PacExecutor, error)` —
  wraps the user-supplied `FindProxyForURL` script body in a small runner
  function and compiles it once with [goja](https://github.com/dop251/goja).
  Returns an error if the script doesn't compile. `p` is a collaborator
  (where `alert()` calls go), kept as its own argument rather than a
  `PacOptions` field, since it's a dependency the executor talks to, not
  a configuration value. `opts` is optional — pass `nil` to use defaults
  for everything.
- `PacOptions` — configuration values for a `PacExecutor`. A `nil` value,
  or a zero-value field, falls back to that field's own default:
  - `DNSTimeout time.Duration` — bounds the DNS builtins (`isResolvable`,
    `dnsResolve`, `isInNet`). Zero means `defaultDNSTimeout` (1s).
  - `ScriptTimeout time.Duration` — bounds a single `Run` call's script
    execution. Zero means `defaultScriptTimeout` (1s).
- `(*PacExecutor).Run(url, host string) (string, error)` — evaluates
  `FindProxyForURL(url, host)` and returns its result (e.g. `"DIRECT"` or
  `"PROXY host:port"`). A script that runs longer than `ScriptTimeout`
  is interrupted (via `goja.Runtime.Interrupt`) and `Run` returns an
  error — without this, an infinite loop in a PAC script (buggy, or
  malicious since PAC content is often fetched from a remote/untrusted
  URL) would hang that call forever.

## Runtime pooling

Each `Run` call needs a goja `*goja.Runtime` (the JS VM instance) to
execute the precompiled `*goja.Program` against. Creating one isn't free,
so `PacExecutor` keeps a `sync.Pool` of them: `Run` takes one from the
pool (or builds a new one via `build()` if the pool is empty) and returns
it when done. The compiled `*goja.Program` itself is immutable and shared
across runtimes. If a script times out, `Run` calls `runtime.ClearInterrupt()`
before returning the runtime to the pool, so the interrupt flag from a
timed-out call can't bleed into the runtime's next reuse.

## Builtin functions (`funcs.go`)

`build()` registers the [standard PAC builtin
functions](https://developer.mozilla.org/en-US/docs/Web/HTTP/Proxy_servers_and_tunneling/Proxy_Auto-Configuration_(PAC)_file)
(`isPlainHostName`, `dnsDomainIs`, `shExpMatch`, `myIpAddress`, etc.) on
the runtime so PAC scripts can call them. They're kept in their own file
since they're a separate concern from the executor/pooling logic in
`pac.go`: plain, mostly stateless string/network helpers with no
knowledge of goja or `PacExecutor`.

The DNS-dependent builtins (`isResolvable`, `dnsResolve`, `isInNet`) take
a `timeout time.Duration` parameter — `build()` binds it from
`PacExecutor.dnsTimeout` via a closure — so a slow or unresponsive DNS
server can't hang script evaluation. `weekdayRange` supports all three
PAC call forms (a single day, a single day with `"GMT"`, or a day range),
since PAC scripts can call it with 1, 2, or 3 arguments.

## IPv6 builtins: the "Ex" family (`funcsex.go`)

The classic builtins above (`isInNet`, `dnsResolve`, `isResolvable`,
`myIpAddress`) are IPv4-only by spec. IPv6 support comes from a separate,
Microsoft-originated extension also supported by Firefox and Chrome,
kept in its own file since it's a distinct (if overlapping) API surface:

- `isInNetEx(ipAddress, ipPrefix)` — like `isInNet`, but `ipPrefix` is
  CIDR notation (`"2001:db8::/32"`) instead of a `pattern, mask` pair, so
  it works for both IPv4 and IPv6 without the fixed-width-integer
  masking `convert_addr` relies on. No DNS lookup — `ipAddress` is
  expected to already be an IP.
- `dnsResolveEx(host)` / `isResolvableEx(host)` — like `dnsResolve` /
  `isResolvable`, but resolve to (or check for) *all* of a host's IPs,
  v4 and v6, semicolon-separated. Bounded by the same
  `PacOptions.DNSTimeout`.
- `myIpAddressEx()` — like `myIpAddress`, but returns every non-loopback
  IP (v4 and v6) configured on the host's interfaces, semicolon
  separated, instead of just the one address the kernel would route
  outbound traffic from.

## Logging (`alert`)

A PAC script's `alert(message)` calls are wired in `build()` to the
`*printer.Printer` injected via `NewPac`, calling `Printer.Infof`. This
replaces kpx's global `logInfo` — there's no global logger in this
project, so the executor needs its own `Printer` reference instead.

## Limitations

- **`dateRange()` and `timeRange()` are unimplemented stubs** — both
  always return `true`, regardless of arguments. A PAC script gating
  proxy use by date or time of day will behave as "always match," not
  fail loudly. Same as in kpx; not part of this migration.
- **`convert_addr()` (and therefore the classic `isInNet()`) is
  IPv4-only** — matches the rest of those builtins, whose
  patterns/masks are always dotted-quad, but it means an IPv6 host
  resolves to `0` (a value indistinguishable from `0.0.0.0`) rather than
  erroring. Use `isInNetEx()` (CIDR notation) for IPv6 matching instead.
- **`sortIpAddressList()`, the remaining "Ex" builtin, is not
  implemented** — only `isInNetEx`, `dnsResolveEx`, `isResolvableEx`,
  and `myIpAddressEx` are. It would need RFC 6724-style address
  preference ordering; PAC scripts rarely call it, so it wasn't worth
  the complexity until one actually needs it.
- **No size/complexity limit on PAC scripts** — only execution *time*
  is bounded (`PacOptions.ScriptTimeout`); a script that's merely slow
  to interrupt-check (e.g. tight loops without function calls) can still
  consume CPU until goja's interrupt check fires.
