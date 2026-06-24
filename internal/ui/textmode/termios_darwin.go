//go:build darwin

package textmode

import "golang.org/x/sys/unix"

// tcget/tcset are the ioctl request numbers for getting/setting termios.
// macOS (BSD-derived) uses TIOCGETA/TIOCSETA, unlike Linux's TCGETS/TCSETS
// — same underlying termios struct, different ioctl encoding.
const (
	tcget = unix.TIOCGETA
	tcset = unix.TIOCSETA
)
