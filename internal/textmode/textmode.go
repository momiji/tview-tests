// Package textmode reads single keypresses from the terminal without
// requiring Enter, so the plain-console UI mode can react to spacebar and
// quit keys while still behaving like a normal scrolling console.
package textmode

import (
	"os"

	"golang.org/x/term"
)

// Signal is the reason Run returned.
type Signal int

const (
	// SwitchToUI means the user pressed space and wants the tview UI.
	SwitchToUI Signal = iota
	// Quit means the user pressed q/Q/Ctrl-C.
	Quit
)

// Run puts stdin into raw mode and blocks, reading one key at a time, until
// the user presses space (SwitchToUI) or q/Q/Ctrl-C (Quit). The terminal is
// always restored to its previous state before Run returns, and normal
// stdout output (e.g. from a printer.Printer) keeps scrolling normally
// while this runs.
func Run() (Signal, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return Quit, err
	}
	defer term.Restore(fd, oldState)

	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			return Quit, err
		}
		switch buf[0] {
		case ' ':
			return SwitchToUI, nil
		case 'q', 'Q', 0x03: // 0x03 is Ctrl-C, which raw mode no longer raises as SIGINT.
			return Quit, nil
		}
	}
}
