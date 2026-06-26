# internal/update

The optional self-update: checks the releases API for a newer version and,
when enabled, downloads and installs the new binary in place. See
[../../ARCHITECTURE.md](../../ARCHITECTURE.md) for how this fits into the rest
of the app.

## API

- `Update(conf *config.ProxyConf, meta Meta, disableAutoRestart bool, p
  *printer.Printer) bool` — runs the check/download/install. Returns `true`
  when the caller should restart the process (the old code exited with code
  200); honors `conf.Check` (skip check), `conf.Update` (skip install) and
  `conf.Restart` (skip restart), plus `disableAutoRestart`.
- `Meta{Name, Version, UpdateUrl}` — the binary name (to pick the OS/arch
  release asset), the current version, and the releases API URL.

## Notes on the port

- The application metadata (`AppName`/`AppVersion`/`AppUpdateUrl`) is passed
  in as `Meta` instead of read from globals; the restart is **signaled** via
  the return value instead of calling `os.Exit(200)`, so the caller controls
  shutdown. Logging goes through the injected `*printer.Printer`.
- `os.Rename` is used instead of `syscall.Rename` for cross-platform install.

## Limitations

- **amd64 only.** The release-asset table only maps `windows/amd64`,
  `linux/amd64` and `darwin/amd64`; other GOOS/GOARCH skip the update
  silently (inherited).
- **No checksum/signature verification** of the downloaded binary (inherited);
  worth adding before relying on it.
