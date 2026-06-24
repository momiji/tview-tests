# internal/textmode

Reads single keypresses from the terminal without requiring Enter, so the
plain-console side of `--ui` mode can react to spacebar and quit keys while
still behaving like a normal scrolling console. See
[../../ARCHITECTURE.md](../../ARCHITECTURE.md) for how this fits into the
rest of the app.

## API

- `Run(switchSignal <-chan struct{}) (Signal, error)` — puts stdin into
  raw mode and waits for either a keypress or `switchSignal` to fire:
  - space, or a closed `switchSignal` → returns `SwitchToUI`
  - `q`, `Q`, or Ctrl-C → returns `Quit`

  Pass `nil` for `switchSignal` to only react to keypresses.

`switchSignal` lets some other part of the program request the switch
asynchronously (e.g. an automatic startup timer owned by `main.go` — see
[../app/README.md](../app/README.md)); `Run` treats it exactly like a
spacebar press and has no notion of why or when it fires.

## Implementation notes

- **Raw input, normal output.** `term.MakeRaw` puts stdin into raw mode
  (no line buffering, no echo, and — critically — no `ISIG`, so Ctrl-C
  arrives as the byte `0x03` instead of a `SIGINT`, letting us treat it the
  same as `q`/`Q`). It also disables output post-processing (`OPOST`),
  which would otherwise mean a plain `\n` no longer gets translated to
  `\r\n` and printed output "stair-steps" down the screen. `Run` puts
  `OPOST`/`ONLCR` back immediately via `unix.IoctlGetTermios`/
  `IoctlSetTermios`, so input stays raw while output behaves normally.
- **Poll loop, not a blocking `Read`.** `Run` polls stdin with
  `golang.org/x/sys/unix.Poll` on a short fixed interval (`pollInterval`)
  rather than issuing one blocking `Read`. This lets it also check
  `switchSignal` regularly, and means that whichever one fires, there is
  never a goroutine left blocked on stdin afterwards — which would
  otherwise race a later call to `Run` reading the same fd.
- The terminal is always restored to its previous state before `Run`
  returns.
