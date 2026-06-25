# internal/config

Turns command-line arguments and a YAML/JSON configuration file into a
resolved, ready-to-use proxy configuration. See
[../../ARCHITECTURE.md](../../ARCHITECTURE.md) for how this fits into the
rest of the app.

This package does **no network and no runtime work**: PAC download and
evaluation, request matching, certificate generation and host caching live
in other packages and run later.

## Three layers

The configuration is modeled as three distinct types instead of one
catch-all struct, matching the `parse → build` pipeline:

- **`CmdArgs`** — command-line arguments (replaces the old global
  `Options`). The `Debug`/`Trace`/`Verbose` booleans are "force on"
  switches: they sit at the top of the cascade below.
- **`FileConfig`** (+ `FileProxy`/`FileRule`/`FileCred`) — exactly what is
  read from the file, nothing more. The yaml/json tags are kept identical to
  the historical format so existing files parse unchanged. The
  verbose/debug/trace/mitm switches are `*bool` tri-states (`nil` = inherit,
  `true`/`false` = explicit).
- **`ProxyConf`** (+ `Proxy`/`Rule`/`Cred`) — the resolved runtime model. The
  switches are plain `bool` (cascade already applied), credentials are
  linked, and the built-in `direct`/`none` proxies and native `kerberos`
  credential are present.

## API

- `Load(args CmdArgs) (*ProxyConf, error)` — the entry point. When
  `args.Config` is empty, the configuration is synthesized from a single
  proxy described on the command line (direct / anonymous / kerberos);
  otherwise the file at `args.Config` is parsed. Then `check` validates it
  and `build` resolves it.

## The tri-state cascade

`verbose` / `debug` / `trace` / `mitm` can be set at several levels. The
effective value is the first **explicit** one, scanning from the **highest**
priority:

```
args (force)  >  file global  >  proxy / rule
```

This is intentionally the inverse of "most specific wins": these are
operator/debug switches, so a higher level always wins and a lower level can
only *add*, never disable. A command-line `-d/-t/-v` therefore always forces
the switch on, preserving the previous CLI behavior. The implication
`trace ⇒ debug ⇒ verbose` is reapplied after the cascade. `mitm` has no
command-line level.

`build` resolves the static levels (args / global / proxy / rule) into plain
booleans on each `Proxy` and `Rule`. The PAC level — the `pac` proxy that
resolved a target at runtime — is applied later, when matching runs.

## Limitations

The following parts of the historical `config.go` are **deliberately not in
this package**; they belong to later migration steps and are tracked in
[../../MIGRATION.md](../../MIGRATION.md):

- **Runtime matching** (`match`, `resolve`, `resolvePac`, the host cache,
  `genPac`, the PAC download) → the `router` package. `build` compiles the
  static derived data here (`Regex` on rules, `PacRegex`/`PacProxy` on
  proxies, the sorted `PacProxies`), but the per-proxy `PacJs`/`PacRuntime`
  are filled in by the router after the PAC download, so the resolved config
  is not fully populated until the router has run.
- **Certificate generation** (`genCerts`, the CA/`Manager` wiring for MITM)
  → the `cert` service and its orchestration.
- **Interactive credential prompts** (`askCredentials`) → the CLI/runtime
  layer (it reads the terminal).
- **PAC-level switch resolution** — the cascade's `pac` level is applied at
  match time, in the router, not here.

The `connection-pools` and `idleTimeout`/`closeTimeout` knobs are dropped on
purpose (see [MIGRATION.md](../../MIGRATION.md) §5): only `connectTimeout`
survives. Old files still using the removed keys keep parsing — the unknown
keys are simply ignored.
