# internal/printer

The dedicated, disableable stdout writer. See
[../../ARCHITECTURE.md](../../ARCHITECTURE.md) for how this fits into the
rest of the app.

## API

- `New() *Printer` — returns a `Printer` that starts enabled.
- `(*Printer).Enable()` / `(*Printer).Disable()` — toggle whether
  `Println` actually writes.
- `(*Printer).Println(a ...any)` — writes to stdout exactly like
  `fmt.Println`, but is a no-op when disabled.

The clock (see [../clock/README.md](../clock/README.md)) always calls
`Printer.Println` on every tick, whether or not output is actually
visible — the printer, not the caller, decides. This is what lets
[../app/README.md](../app/README.md) silence plain-console output while
the tview UI is showing, and re-enable it when switching back, without the
clock's own logic ever branching on mode.
