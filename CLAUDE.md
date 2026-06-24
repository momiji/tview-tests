# Project instructions

- Documentation is split by altitude: `ARCHITECTURE.md` stays high-level
  (run modes, the overall core idea, and a one-line-per-package index
  linking to each package's README). Detailed, package-specific design
  (API, implementation notes, rationale) lives in that package's own
  `README.md` (e.g. `internal/service/clock/README.md`,
  `internal/ui/README.md`). Whenever code changes are made, update the
  relevant package's `README.md` as part of the same change, and update
  `ARCHITECTURE.md` too if the change affects overall run-mode behavior,
  adds/removes a package, or otherwise changes the high-level picture.
- Do not run the app yourself to test it (e.g. via pty/python harnesses).
  The user runs it manually (it's an interactive TUI) and reports results.
  Stick to `go build`/`go vet` to confirm it compiles.
