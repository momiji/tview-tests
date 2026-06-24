// Package textmode reads single keypresses from the terminal without
// requiring Enter, so the plain-console UI mode can react to spacebar and
// quit keys while still behaving like a normal scrolling console.
package textmode

import (
	"os"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// Signal is the reason Run returned.
type Signal int

const (
	// SwitchToUI means something asked to move to the tview UI: either the
	// user pressed space, or switchSignal (see Run) fired.
	SwitchToUI Signal = iota
	// Quit means the user pressed q/Q/Ctrl-C.
	Quit
)

// pollInterval bounds how quickly Run notices that switchSignal fired,
// since that channel can only be checked in between poll() calls.
const pollInterval = 100 // milliseconds

// Run puts stdin into raw mode and reads one key at a time until the user
// presses space, or until switchSignal fires — both return SwitchToUI.
// switchSignal lets some other part of the program request the switch
// asynchronously (e.g. an automatic startup timer); pass nil to disable it
// and only react to keypresses. q/Q/Ctrl-C always return Quit.
//
// Waiting is done with a poll loop (rather than a single blocking Read) so
// switchSignal can be checked regularly and so that, on return, there is no
// goroutine left blocked on stdin — the next caller can safely read from
// stdin again without racing this one.
//
// The terminal is always restored to its previous state before Run
// returns, and normal stdout output (e.g. from a printer.Printer) keeps
// scrolling normally while this runs.
func Run(switchSignal <-chan struct{}) (Signal, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return Quit, err
	}
	defer term.Restore(fd, oldState)

	// term.MakeRaw also disables output post-processing (OPOST/ONLCR), so a
	// plain '\n' from the printer would no longer be translated to '\r\n'
	// and output would "stair-step" down the screen. Put that back: we only
	// need raw *input* (no line buffering/echo, and no ISIG so Ctrl-C
	// reaches us as a byte instead of a signal), not raw output.
	if t, err := unix.IoctlGetTermios(fd, unix.TCGETS); err == nil {
		t.Oflag |= unix.OPOST | unix.ONLCR
		unix.IoctlSetTermios(fd, unix.TCSETS, t)
	}

	buf := make([]byte, 1)
	for {
		select {
		case <-switchSignal:
			return SwitchToUI, nil
		default:
		}

		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, pollInterval)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return Quit, err
		}
		if n == 0 {
			continue // poll timed out; loop around to recheck switchSignal
		}

		nRead, err := os.Stdin.Read(buf)
		if err != nil || nRead == 0 {
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
