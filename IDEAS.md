# Ideas

Adaptations, fixes and optimizations spotted *during* the `kpx/` → `internal/`
migration but deliberately **not** applied while porting (the copy stays
faithful — see [MIGRATION.md](MIGRATION.md) §6 and `CLAUDE.md`). Each entry:
component + idea + why (+ a code pointer when useful). Revisit these after the
port is stabilized.

## general idea

- Check if project is now usable as a library, and identify what needs to be enhanced to do so, like changeing the printer fmp.Printf with a customizable Println func and a default printer implement that just do ft.Print
- Add an option to cache pac js, userfull maybe for servers usage?
- add ha to rule, pac, proxy, global: for roundrobin, first, faste, upstraem > service ?

## internal/service/secret (step 1, from kpx/password.go)

- **Cache the derived key/hash.** `Encrypt`/`Decrypt` re-read the key file and
  recompute the MD5 hash on every call (`createHash` → `readKey`). The key file
  is effectively immutable for a process lifetime, so derive it once in `New`.
- **Harmonize error handling.** `Encrypt` and the internal key read `panic` on
  failure, while `Decrypt` returns an `error`. Make the API consistent (likely
  return errors everywhere, let callers decide).
- **Reconsider MD5 key derivation.** Deriving the AES-256 key via MD5 of the
  key file is weak. Kept for backward compatibility with existing key files;
  evaluate a stronger KDF (with a migration path) later.

## internal/config (step 2, from kpx/config.go)

- **Immutability of `ProxyConf`.** Per MIGRATION.md §2, the resolved config
  should be fully immutable after `build`. Today `markUsed` mutates
  `rule.Proxy` (dns-only rules → `direct`) and routing-derived fields will be
  filled in later (step 7). When routing lands, audit every field and make
  the post-build object read-only (resolve dns→direct inside `buildRule`,
  compute regex/pac artifacts before publishing).
- **Per-(rule,proxy) switch resolution.** The cascade is resolved per entity
  (proxy and rule separately) and combined at runtime. Once the processor
  lands, confirm the combination order (proxy vs rule vs pac) matches §4.3
  and consider precomputing the effective value per resolved decision.
- **`build` re-reads the key file per encrypted password.** `secret.Cipher`
  is created once now (good), but it still recomputes the key/hash on each
  `Decrypt`; ties into the secret-package caching idea above.

## internal/service/cert (step 3, from kpx/certs.go + certs_manager.go)

- **`NewManager` swallows an error in the default branch.** In the preload
  loop, the final `else` assigns `certs.certificates[dns], err =
  newCertificate(dns)` but does not check `err` (unlike the `*.` branch).
  A failure there is silently ignored. Preserved as-is during the port;
  worth tightening.
- **Hardcoded 2048-bit RSA / fixed validity windows.** Key size and the
  CA/leaf NotAfter (100y / 10y) are baked in. Consider making them
  configurable.

## internal/service/kerberos (step 4, from kpx/kerberos*.go)

- **Shared `splitUsername`/`splitHostPort`.** Local copies now live in both
  `config` and `kerberos`; fold them into a shared util when the CLI lands.
- **App-level defaults parked here.** `DefaultDomain`/`DefaultKrb5` (and the
  similar `AppName`/`AppVersion`/... globals still in the legacy source) want
  a dedicated app/meta home rather than living in this service.
- **`NewWithPassword` copies krb5 config via `&(*k.krbCfg)`.** Shallow copy
  of a struct holding slices/maps; the realm append mutates the copy but the
  sharing is subtle. Audit for races when the processor runs it concurrently.
- **SPNEGO clients with no TTL/refresh.** Cached clients in `Store` are never
  evicted except on token failure (forced re-login). Consider ticket
  lifetime handling.

## internal/proxy/transport (step 5, from kpx/conn.go + chunked.go)

- **UI reconciliation (step 14).** The UI's `TrafficRow` must implement
  `transport.TrafficMeter` (`AddReceived`/`AddSent`), or an adapter must,
  when the traffic UI is migrated. The metering currently has no
  implementation wired.
- **Header-read timeout in the processor.** When the processor is migrated,
  bound the request-header read with a one-shot
  `conn.SetReadDeadline(now + connectTimeout)` (the replacement for the
  dropped `TimedConn`); make sure it is cleared afterwards.

## internal/proxy/message (step 6, from kpx/request.go)

- **Export surface is provisional.** The read/write methods and all
  `RequestHeader` fields were exported so the (not-yet-migrated) processor
  can use them. Revisit once the processor lands: tighten what does not need
  to be public, and consider getters over exported mutable fields.
- **`splitHostPort` duplicated** (config, kerberos, message). Fold into a
  shared util at the CLI step.
- **Mixed pointer/value receivers** kept from the original; the value
  receivers on the write methods are harmless (no mutation) but inconsistent.

## internal/proxy/router (step 7, from kpx/config.go matching)

- **Router mutates the resolved config.** `NewRouter` writes `PacJs`/
  `PacRuntime` onto `config.Proxy`. To make `ProxyConf` truly immutable,
  consider holding the per-proxy PAC runtimes in a router-owned map keyed by
  proxy name, rather than on the shared config struct.
  Or maybe by proxy url, because I can change the proxy url and keep the same name and config will reload but the js will not be refreshed?
- **`errors.New(fmt.Sprintf(...))`** in `downloadPac` should be `fmt.Errorf`
  (kept verbatim from the original).
- **Persistent PAC js cache.** `pacsCache` is in-memory only; a used pac that
  fails to download with no cached copy degrades to the default proxy. A
  disk-backed cache would help server usage (see the "cache pac js" general
  idea above).
- **Host cache never invalidated.** Entries live for the router's lifetime;
  a config reload builds a new router, but long-lived routers never expire
  stale host→proxy decisions.

## internal/proxy/upstream (step 8, from kpx/process.go findFirstProxy)

- **In-place sort aliases the caller's slice.** `append(proxies[:0],
  proxies...)` + `sort` reorders the router's cached proxy slice. Copy before
  sorting to avoid the shared-state hazard.
- **`proxyShortName` duplicated** (also needed by the processor); share it.
- **HA state never trimmed.** `lastProxies` grows with proxy/host keys and is
  never cleaned; fine for small configs, revisit for churny ones.

## internal/proxy/processor (step 9, from kpx/process.go)

- **Diagnostic header rename.** The debug-mode response headers (old product-
  prefixed `*-proxy`/`*-host`) are now `x-proxy-name`/`x-proxy-host` to avoid
  a product name literal in code. Once the app name/meta is migrated, tie the
  prefix to it (configurable) and restore the intended names.
- **`authorizationContext` is dead without the pool.** The per-conf/per-user
  auth still computes an `authorizationContext` (was the pool key); with the
  pool gone it is unused (`_ = authorizationContext`). Drop it from the auth
  signatures once forward/socks are ported.
- **`splitHostPort`/`proxyShortName` duplicated** again here; fold into a
  shared util at the CLI step.
- **Traffic rows are not registered.** `TrafficRow` implements the meter but
  is not added to a table; wire it to the UI table at step 14 (and deregister
  on close).

## internal/config watch + processor reload (step 11)

- **Reload feasibility lives in the app callback.** `config.Watch` is generic
  (trigger on change/periodic); the mod-time gating, credential hot-reload
  check (old kpx `reload()`), and `runtime.Reload` publishing are wired in the
  app layer (step 13). Make sure the callback honors `router.NeedFastReload()`.
- **Kerberos/cert not swapped on reload.** Only conf/router/selector are
  republished; the kerberos store and cert manager are kept across reloads
  (as before). Revisit if krb5 config or MITM rules change on reload.

## internal/update (step 12, from kpx/main.go)

- **No checksum/signature verification** of the downloaded binary; add before
  trusting auto-update.
- **amd64-only asset table**; extend to arm64 etc.

## internal/cli + internal/app (step 13)

- **Self-update restart no longer exits with code 200.** The old restarter
  contract (exit 200) became a graceful runtime.Stop(); reintroduce an exit
  code if an external supervisor relies on it.
- **In-place config mutation at load/reload.** askCredentials and the reloader
  set Login/Password on the resolved config; ties into the ProxyConf
  immutability goal.
- **Console UI (`--ui`) parsed but not wired** until the UI reconciliation
  (step 14); the proxy runs in plain logging mode.

## internal/ui/traffic (step 14a)

- **Keep-alive rows leak.** Each processChannel iteration creates a new
  TrafficRow but only the last is Removed on close; intermediate keep-alive
  rows are never marked Removed, so RemoveDead never drops them. Faithful to
  the original; fix by reusing one row per connection or removing on each
  iteration.

## internal/ui (step 14b)

- **Text/UI live toggle restored.** `--ui` starts in the scrolling proxy
  log and toggles to the tview traffic table with space (`internal/ui`
  `RunUI` + the `internal/ui/textmode` raw-mode reader, driving the
  printer's `Disable`/`Enable`). The Windows reader (`run_windows.go`) is a
  best-effort placeholder to be replaced with a known-good implementation.
- **Old demo packages removed.** The clock/tui demo and service/clock were
  deleted as superseded by the proxy; the textmode reader was kept and
  rewired to drive the traffic table.
