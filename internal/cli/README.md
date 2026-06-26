# internal/cli

Parses the command-line flags into a `config.CmdArgs` and decides which
action to run. Owns the usage/help/version text. See
[../../ARCHITECTURE.md](../../ARCHITECTURE.md) for how this fits into the rest
of the app.

## API

- `Parse(meta Meta) (config.CmdArgs, Action, error)` — parses `os.Args` into a
  resolved `CmdArgs` (config path defaulted to `<name>.yaml`/`.json`, or the
  single-proxy positional argument expanded into bind/proxy/login/domain) and
  an `Action` (`Run` / `Help` / `Version` / `Encrypt`). It does not exit.
- `EncryptPassword(args)` — the `-e` command: prompts for a password and
  prints `secret.Prefix` + the encrypted value (using `args.KeyFile`).
- `Usage(meta)` / `Help(meta)` / `Version(meta)` — the rendered text.
- `Meta{Name, Version, Url, UpdateUrl, DefaultDomain}` — application metadata,
  injected from the binary so this package has no hard-coded product name.

## Notes on the port

- Replaces the global `Options` + `flag` package-level state with a
  `flag.FlagSet` filling a `config.CmdArgs`; the implication
  `trace ⇒ debug ⇒ verbose` is applied later in `config.build`, not here.
- The help's config sample was trimmed of the dropped knobs
  (`idleTimeout`/`closeTimeout`, `connection-pools`).
- `splitHostPort`/`splitUsername` are local copies (also in config/kerberos/
  message); a shared util is still pending (IDEAS.md).
