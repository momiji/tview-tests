# internal/ui

The optional interactive terminal UI, shown when the proxy is started with
`--ui`. It starts in plain-console text mode and lets the user toggle live
between the scrolling proxy log and a `tview` traffic table. See
[../../ARCHITECTURE.md](../../ARCHITECTURE.md) for how this fits into the
rest of the app.

## API

- `RunUI(ctx context.Context, data *traffic.TrafficTable, p *printer.Printer) error`
  — the `--ui` entry point. It starts in **text mode** (the `printer` keeps
  scrolling the proxy log) and switches to the **traffic table** when the
  user presses space; from the table, space returns to text mode, so the
  two can be toggled indefinitely. `q`/`Q`/Ctrl-C quits from either mode,
  and a cancelled `ctx` (proxy shutdown) unblocks whichever mode is on
  screen. It blocks until quit/shutdown and always leaves the terminal back
  in plain console state.
- `RunTraffic(ctx context.Context, data *traffic.TrafficTable) (Signal, error)`
  — renders just the table: it takes over the terminal and refreshes a
  table of the current connections a few times per second (request id, URL,
  bytes received/sent and the per-second rates), colored by state (active /
  stalled / removed). It returns `SwitchToText` on space, or `Quit` on
  `q`/`Q`/Ctrl-C or a cancelled `ctx`. `RunUI` calls it; it can also be used
  directly to show only the table.

The caller runs the proxy server in the background. `RunUI` disables the
async `printer` while the table is on screen and re-enables it on the way
back to text mode. The traffic data itself lives in
[traffic/](traffic/README.md), fed by the proxy regardless of the UI.

Single-keypress, raw-mode terminal input for text mode (space to switch,
`q`/`Q`/Ctrl-C to quit, while normal log output keeps scrolling) is handled
by the [textmode/](textmode/README.md) sub-package.
