# internal/proxy/message

Reads, parses and writes the HTTP messages exchanged by the proxy. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits into
the rest of the app.

The parser is deliberately raw: it keeps the original header lines as
strings (rewriting only the `Host:` line when it has to absolutize a URL)
instead of decoding into `net/http` types, so messages can be forwarded
verbatim, including quirks the standard library would normalize away.

## API

- `ProxyRequest` — wraps one connection. `NewProxyRequest(conn, printer,
  prefix)` builds it; a non-empty `prefix` enables header logging through the
  injected `*printer.Printer` (`ReqHeaderf`, which redacts
  `Proxy-Authorization`).
  - `ReadRequestHeaders()` / `ReadResponseHeaders(allowEOFDelimitedBody)` —
    read and parse the headers, populating `Header`.
  - `InjectResponseHeaders(headers)` — parse a response from header lines
    already in memory (no read).
  - `FindHeader(name)` — first matching header value, or nil.
  - `WriteRequestLine` / `WriteStatusLine` / `WriteHeader` / `WriteKeepAlive`
    / `WriteContent` / `CloseHeader` / `WriteHeaderLine` — write the outgoing
    message; `BadRequest` / `NotFound` / `RequireAuth` are canned responses.
- `RequestHeader` — the parsed result: raw `Headers`, leftover body `Data`,
  and the extracted fields (`Method`, `Url`, `Host`, `Port`, `HostPort`,
  `IsConnect`, `IsSsl`, `KeepAlive`, `ContentLength`, `DirectToConnect`, the
  response `Status`/`Reason`, ...).
- `HttpVersion` (`Http10`/`Http11`/`Http2`) with `GetHttpVersion`, `Version`,
  `Order`.

## Notes on the port

- `ProxyRequest.conn` is a plain `net.Conn` (the dropped `TimedConn` wrapper
  is gone, MIGRATION.md §5). Header logging goes through the injected
  `*printer.Printer` instead of the old global logger.
- The read/write methods and the `RequestHeader` fields are exported because
  the request processor lives in a separate package now; the parsing logic is
  otherwise unchanged.

## Limitations

- **`splitHostPort` is a local copy** (also present in `config` and
  `kerberos`); to be folded into a shared util when the CLI is migrated.
- **`analyseHeaders` is request/response and quirk heavy.** It special-cases
  the `/~/http(s)/...` direct-to-CONNECT upgrade and `Host: http/...` forms;
  these paths are ported as-is and want dedicated tests (see
  [../../../MIGRATION.md](../../../MIGRATION.md) §7).
