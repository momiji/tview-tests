# Project instructions

- Whenever code changes are made in this repo, update `ARCHITECTURE.md` to
  reflect them (new packages, changed responsibilities, changed mode
  behavior, etc.) as part of the same change, not as a follow-up.
- Do not run the app yourself to test it (e.g. via pty/python harnesses).
  The user runs it manually (it's an interactive TUI) and reports results.
  Stick to `go build`/`go vet` to confirm it compiles.
