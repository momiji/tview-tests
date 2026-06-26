# internal/app

Wires everything together and runs the proxy. See
[../../ARCHITECTURE.md](../../ARCHITECTURE.md) for the high-level picture.

## API

- `Main(meta cli.Meta) int` — the program entry point (called from
  `cmd/kpx`). It sets up a signal-cancellable context and the async
  `printer`, parses the CLI, and dispatches the action: print help/version,
  encrypt a password, or `run` the proxy. Returns the process exit code.

## What `run` wires

1. `config.Load(args)` → resolved `ProxyConf`.
2. `askCredentials` — interactive prompts for missing login/password.
3. `genCerts` — builds the `cert.Manager` when a rule uses MITM (load/create
   the CA from `<name>.ca.crt`/`.ca.key`).
4. `kerberos.NewStore` + `loadKerberos` — initial login for used kerberos
   proxies (password or native OS).
5. `router.NewRouter` + `upstream.NewSelector`.
6. `processor.NewRuntime` — bundles the above behind an atomic snapshot.
7. background tasks: auto-exit timeout, `config.Watch` → `reloader.reload`
   (hot-reload with credential feasibility), and the self-`update` loop.
8. `server.New(...).Run(ctx)` — the listeners, blocking until shutdown.

Shutdown is by context cancellation: a signal (SIGINT/SIGTERM), the
auto-exit timeout, or a self-update restart all cancel the runtime context,
which stops the listeners and the background tasks.

## Limitations

- **Console UI not wired.** `--ui` / `ui: true` is parsed but the console/
  tview UI is reconciled in a later step (MIGRATION.md step 14); the proxy
  currently runs in plain logging mode.
- **Self-update restart exits via shutdown, not code 200.** The old behavior
  exited with code 200 to signal an external restarter; here a successful
  self-update just stops the runtime. Revisit if an exit-code contract is
  needed.
- **`askCredentials`/reload mutate the resolved config in place** (setting
  `Login`/`Password`); acceptable at load time but noted against the
  immutability goal (IDEAS.md).
