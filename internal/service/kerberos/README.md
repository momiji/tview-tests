# internal/service/kerberos

Authenticates upstream proxies with SPNEGO/Negotiate tokens. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits into
the rest of the app.

Two authentication paths:

- **Password-based** — `Store` keeps a cache of logged-in clients keyed by
  `(password-hash, username, realm)` and hands out base64 SPNEGO tokens for
  a given `protocol/host` SPN. It derives the realm from the username
  (`DOMAIN\user` / `user@domain`), expands a dotless realm with
  `DefaultDomain`, and probes/explodes KDC host lists to find a reachable
  one (cached for `KDCTestTimeout` seconds).
- **Native OS single-sign-on** — `NativeKerberos` uses the credentials the
  OS already holds: the ccache on Linux, SSPI on Windows; other OSes return
  an error. It needs no password.

## API

- `NewStore(conf *config.ProxyConf, p *printer.Printer) (*Store, error)` —
  builds the krb5 configuration (from `conf.Krb5` or `DefaultKrb5`) and the
  client cache. Reads `conf.Domains`, `conf.ConnectTimeout`.
- `(*Store).safeGetToken(username, realm, password, protocol, host)` —
  package-internal entry point used by the proxy processors (exported
  surface will firm up when the processor lands).
- `NativeKerberos.SafeTryLogin(p *printer.Printer) error` and
  `NativeKerberos.SafeGetToken(protocol, host)` — the native path.

## Notes on the port

- The whole resolved `config.ProxyConf` is injected instead of the old
  global config; only `Krb5`, `Domains` and `ConnectTimeout` are read.
- Logging goes through the injected `*printer.Printer` (`Infof`) — there is
  no global logger. `SafeTryLogin` takes the printer as an argument because
  `NativeKerberos` is a per-OS package singleton with no constructor.
- `DefaultDomain` / `DefaultKrb5` are package vars (overridable by a build),
  ported from the old app-level globals; `KDCTestTimeout` is a const.

## Limitations

- **`splitUsername` / `splitHostPort` are local copies.** They also exist in
  `config` (and will exist in the CLI layer); to be reconciled into a shared
  util when the CLI is migrated.
- **`DefaultDomain` / `DefaultKrb5` placement is provisional.** They are
  app-level defaults parked here for now; they may move to an app/meta
  package once the CLI and app wiring are migrated.
