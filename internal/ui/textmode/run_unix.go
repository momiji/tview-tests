//go:build !windows

package textmode

import (
	"context"
	"os"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// pollInterval bounds how quickly Run notices that switchSignal fired or
// that ctx was cancelled, since both can only be checked in between poll()
// calls.
const pollInterval = 100 // milliseconds

// Run puts stdin into raw mode and reads one key at a time until the user
// presses space, or until switchSignal fires — both return SwitchToUI.
// switchSignal lets some other part of the program request the switch
// asynchronously (e.g. an automatic startup timer); pass nil to disable it
// and only react to keypresses. q/Q/Ctrl-C, or a cancelled ctx, return Quit.
//
// Waiting is done with a poll loop (rather than a single blocking Read) so
// switchSignal and ctx can be checked regularly and so that, on return,
// there is no goroutine left blocked on stdin — the next caller can safely
// read from stdin again without racing this one.
//
// The terminal is always restored to its previous state before Run
// returns, and normal stdout output (e.g. from a printer.Printer) keeps
// scrolling normally while this runs.
func Run(ctx context.Context, switchSignal <-chan struct{}) (Signal, error) {
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
	// reaches us as a byte instead of a signal), not raw output. tcget/tcset
	// are the OS-specific ioctl request numbers (see termios_*.go).
	if t, err := unix.IoctlGetTermios(fd, tcget); err == nil {
		t.Oflag |= unix.OPOST | unix.ONLCR
		unix.IoctlSetTermios(fd, tcset, t)
	}

	buf := make([]byte, 1)
	for {
		select {
		case <-ctx.Done():
			return Quit, nil
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
			continue // poll timed out; loop around to recheck ctx/switchSignal
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
