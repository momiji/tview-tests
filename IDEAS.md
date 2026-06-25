# Ideas

Adaptations, fixes and optimizations spotted *during* the `kpx/` → `internal/`
migration but deliberately **not** applied while porting (the copy stays
faithful — see [MIGRATION.md](MIGRATION.md) §6 and `CLAUDE.md`). Each entry:
component + idea + why (+ a code pointer when useful). Revisit these after the
port is stabilized.

## general idea

- Check if project is now usable as a library, and identify what needs to be enhanced to do so, like changeing the printer fmp.Printf with a customizable Println func and a default printer implement that just do ft.Print

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
