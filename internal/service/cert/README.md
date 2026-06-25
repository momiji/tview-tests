# internal/service/cert

The X.509 building blocks for MITM: a self-signed CA and per-host leaf
certificates signed by it, used to terminate inbound TLS. See
[../../../ARCHITECTURE.md](../../../ARCHITECTURE.md) for how this fits into
the rest of the app.

## API

- `Cert` — an RSA key pair plus its parsed certificate.
- `NewCert(template, bits, ca)` — generates a key and a certificate from an
  `x509.Certificate` template, self-signed when `ca` is nil, otherwise
  signed by `ca`.
- `NewBasicCACertConfig(cn, serial)` / `NewBasicHttpsCertConfig(cn, names,
  serial)` — ready-made templates for a CA and for a leaf HTTPS server
  certificate (the latter splits `names` into IPs and DNS SANs).
- `NewCertFromPEM` / `NewCertFromFiles` — load an existing cert/key pair.
- `(*Cert).ToPEM()` / `(*Cert).SaveToFiles(pub, priv)` — serialize.
- `Manager` — caches leaf certificates per host and mints them lazily.
  `NewManager(ca, prefix, names)` preloads `names`: a `*.` prefix gets a
  wildcard cert eagerly, while `**`/`**.` entries are markers minted on
  first matching request. `GetCertificate(dns)` returns the cached cert or
  mints one, walking from exact host to `*.domain` to `**` wildcards. It is
  safe for concurrent use (read lock fast path, write lock to mint).

## Limitations

- **CA orchestration is not here.** Reading/creating the CA files and
  deciding *whether* MITM is enabled (scanning rules for `mitm`) is wiring
  that ties this service to the resolved configuration; it lives in the
  app/processor layer and is ported with a later step. This package only
  provides the certificate primitives and the cache.
- **PBKDF2 deterministic key generation dropped.** The historical
  `NewPbkdfCert` / `RandomReader` (deriving an RSA key deterministically
  from a password via PBKDF2) were dead code — never called — and are not
  ported, which also avoids the `golang.org/x/crypto/pbkdf2` dependency. If
  reproducible CA keys are ever needed, reintroduce them deliberately.
