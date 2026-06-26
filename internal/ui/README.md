# internal/ui

The optional live traffic table, rendered with `tview` when the proxy is
started with `--ui`. See [../../ARCHITECTURE.md](../../ARCHITECTURE.md) for how
this fits into the rest of the app.

## API

- `RunTraffic(ctx context.Context, data *traffic.TrafficTable) error` — takes
  over the terminal and refreshes a table of the current connections a few
  times per second (request id, URL, bytes received/sent and the per-second
  rates), colored by state (active / stalled / removed). It blocks until the
  context is cancelled (proxy shutdown) or the user quits with `q`/`Q`/Ctrl-C.

The caller runs the proxy server in the background and disables the async
`printer` first (the UI owns the screen). The traffic data itself lives in
[traffic/](traffic/README.md), fed by the proxy regardless of the UI.

## Limitations

- **No text/UI toggle.** The original could switch live between plain log
  output and the table (space); this port only offers "table while `--ui`,
  plain logs otherwise". The toggle and its suspendable log writer are not
  ported.
