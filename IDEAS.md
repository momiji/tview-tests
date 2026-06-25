# Ideas

Adaptations, fixes and optimizations spotted *during* the `kpx/` → `internal/`
migration but deliberately **not** applied while porting (the copy stays
faithful — see [MIGRATION.md](MIGRATION.md) §6 and `CLAUDE.md`). Each entry:
component + idea + why (+ a code pointer when useful). Revisit these after the
port is stabilized.

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
