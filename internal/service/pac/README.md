# internal/service/pac

Evaluates PAC (Proxy Auto-Configuration) scripts to decide, for a given
URL/host, what proxy (if any) to use. Migrated from kpx's `pac.go`. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits
into the rest of the app.

## API

- `NewPac(pacJs string, p *printer.Printer) (*PacExecutor, error)` —
  wraps the user-supplied `FindProxyForURL` script body in a small runner
  function and compiles it once with [goja](https://github.com/dop251/goja).
  Returns an error if the script doesn't compile.
- `(*PacExecutor).Run(url, host string) (string, error)` — evaluates
  `FindProxyForURL(url, host)` and returns its result (e.g. `"DIRECT"` or
  `"PROXY host:port"`).

## Runtime pooling

Each `Run` call needs a goja `*goja.Runtime` (the JS VM instance) to
execute the precompiled `*goja.Program` against. Creating one isn't free,
so `PacExecutor` keeps a `sync.Pool` of them: `Run` takes one from the
pool (or builds a new one via `build()` if the pool is empty) and returns
it when done. The compiled `*goja.Program` itself is immutable and shared
across runtimes.

## Builtin functions (`funcs.go`)

`build()` registers the [standard PAC builtin
functions](https://developer.mozilla.org/en-US/docs/Web/HTTP/Proxy_servers_and_tunneling/Proxy_Auto-Configuration_(PAC)_file)
(`isPlainHostName`, `dnsDomainIs`, `shExpMatch`, `myIpAddress`, etc.) on
the runtime so PAC scripts can call them. They're kept in their own file
since they're a separate concern from the executor/pooling logic in
`pac.go`: plain, mostly stateless string/network helpers with no
knowledge of goja or `PacExecutor`. `dateRange`/`timeRange` are stubs
that always return `true` (not implemented, same as in kpx).

## Logging (`alert`)

A PAC script's `alert(message)` calls are wired in `build()` to the
`*printer.Printer` injected via `NewPac`, calling `Printer.Infof`. This
replaces kpx's global `logInfo` — there's no global logger in this
project, so the executor needs its own `Printer` reference instead.
