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

## Per-OS implementations

`Run` has the same signature and behavior everywhere, but no single
implementation works on Linux, macOS, and Windows, so it's split across
build-tagged files:

- **`run_unix.go`** (`!windows`, i.e. Linux and macOS) — the poll-based
  implementation:
  - **Raw input, normal output.** `term.MakeRaw` puts stdin into raw mode
    (no line buffering, no echo, and — critically — no `ISIG`, so Ctrl-C
    arrives as the byte `0x03` instead of a `SIGINT`, letting us treat it
    the same as `q`/`Q`). It also disables output post-processing
    (`OPOST`), which would otherwise mean a plain `\n` no longer gets
    translated to `\r\n` and printed output "stair-steps" down the screen.
    `Run` puts `OPOST`/`ONLCR` back immediately via
    `unix.IoctlGetTermios`/`IoctlSetTermios`, so input stays raw while
    output behaves normally.
  - **Poll loop, not a blocking `Read`.** `Run` polls stdin with
    `golang.org/x/sys/unix.Poll` on a short fixed interval (`pollInterval`)
    rather than issuing one blocking `Read`. This lets it also check
    `switchSignal` regularly, and means that whichever one fires, there is
    never a goroutine left blocked on stdin afterwards — which would
    otherwise race a later call to `Run` reading the same fd.
  - **`termios_linux.go` / `termios_darwin.go`** supply the `tcget`/`tcset`
    ioctl request constants the OPOST/ONLCR fix needs. Linux uses
    `TCGETS`/`TCSETS`; macOS (BSD-derived) uses `TIOCGETA`/`TIOCSETA`
    instead — same termios struct, different ioctl encoding — which is why
    these live in their own per-OS files rather than in `run_unix.go`
    itself.
- **`run_windows.go`** — Windows has no equivalent of POSIX `poll()` on
  stdin to bound a blocking `Read` with a timeout/cancel, so the poll-loop
  approach above doesn't translate. Instead, a single goroutine
  (`startReader`, started once via `sync.Once`) reads stdin byte-by-byte
  for the entire life of the process and publishes each byte to a shared
  channel; every call to `Run` selects on that channel and on
  `switchSignal`. Sharing one persistent reader avoids ever having two
  goroutines racing a blocked `Read` for the same keypress, which a
  per-call reader (started and abandoned on every `Run` call) would risk
  on a platform where the read can't be cancelled.

In all cases, the terminal is always restored to its previous state before
`Run` returns.
