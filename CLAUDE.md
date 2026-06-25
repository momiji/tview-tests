# Project instructions

- Language: all documentation and code (including comments, identifiers,
  commit messages, and `README.md`/`ARCHITECTURE.md` content) must be
  written in English. Only chat replies to the user may be in French.
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
- Every package `README.md` that has any known gaps — unimplemented
  functions/stubs, partial spec support, deliberately out-of-scope
  behavior — must call them out in a dedicated `## Limitations` section,
  so they're easy to find rather than buried inline in prose elsewhere
  in the doc (see `internal/service/pac/README.md` for the pattern).
  When code changes close a limitation, remove or update its entry in
  the same change; when new code introduces one, add it there too.

## Migrating from kpx/

`kpx/` is legacy source being ported into this project's `internal/`
layout one file (or one logical class/concern) at a time. For each item,
follow this two-phase process — do not skip straight to copying code:

1. **Propose a split, then stop and wait for validation.** Before writing
   any code, read the source file and present:
   - Which destination package(s)/file(s) each piece of functionality
     should land in — an existing package extended with a new file, a new
     package, or a new file alongside an existing one (e.g. a `request.go`
     next to a `printer.go` when one file mixes two concerns).
   - Which methods/types map to which destination, named explicitly.
   - Which methods are candidates to drop entirely because the new
     architecture already covers them (e.g. manual `*Init`/`*Destroy`/
     lifecycle plumbing that's superseded by this project's
     context-cancellation-based shutdown), and why.
   - Do not write or move any code in this step — text only, then ask the
     user to confirm or adjust the split.
2. **Once approved, copy with minimal transformation.** Port the
   validated pieces largely as-is (logic, structure, names) into their new
   home — adapt only what's mechanically required to fit the destination
   (package name, imports, adapting to this project's existing types like
   `Printer` instead of re-inventing them). Resist the urge to redesign,
   rename for style, or clean up while moving; that's a separate, later
   step if wanted. Update the destination package's `README.md` (and
   `ARCHITECTURE.md` if the split adds/removes a package) per the
   documentation rule above.
