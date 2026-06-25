# internal/service/secret

Encrypts and decrypts configuration passwords with a locally stored key
file. It is a leaf service: pure crypto, no network, no dependency on other
internal packages.

An AES-256 key is derived by taking the MD5 hex digest of the key file's
bytes; payloads are sealed with AES-GCM (random nonce prepended) and
base64-encoded. A configuration value is recognized as encrypted when it
starts with `Prefix`.

## API

- `New(keyFile string) *Cipher` — returns a `Cipher` bound to `keyFile`.
  The file is read lazily on the first `Encrypt`/`Decrypt`; if it does not
  exist, a fresh random 256-byte key is generated and written with `0600`
  permissions.
- `(*Cipher).Encrypt(data string) string` — returns the base64-encoded
  ciphertext, **without** `Prefix`. Callers that store the result in config
  prepend `Prefix` themselves.
- `(*Cipher).Decrypt(data string) (string, error)` — reverses `Encrypt`;
  `data` must be the ciphertext with `Prefix` already stripped.
- `Prefix` (`"encrypted:"`) — marks a value as encrypted in config files.

The key file location is injected through `New` rather than read from a
global, keeping the domain free of CLI/global state.

## Limitations

- **Key/hash recomputed on every call.** `Encrypt`/`Decrypt` re-read the
  key file and recompute the MD5 hash each time, instead of caching it in
  `New`. Noted as a future improvement in `IDEAS.md`.
- **`panic` on key-write / GCM-setup failure.** `Encrypt` (and the internal
  key read) panic on failure, whereas `Decrypt` returns an `error`.
  Harmonizing this asymmetry is deferred (`IDEAS.md`).
- **MD5-based key derivation.** Using MD5 to derive the AES key is weak by
  modern standards; preserved for backward compatibility with existing key
  files, flagged for later review (`IDEAS.md`).
- **Interactive password-encryption command not included.** The command
  that prompts for a password and prints its encrypted form is a CLI
  concern and will live in `internal/cli`; it will use `Encrypt` together
  with `Prefix`.
