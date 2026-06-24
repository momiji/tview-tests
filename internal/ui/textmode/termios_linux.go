//go:build linux

package textmode

import "golang.org/x/sys/unix"

// tcget/tcset are the ioctl request numbers for getting/setting termios.
// They differ between Linux and the BSD family (including macOS), which is
// why they live in their own per-OS file instead of run_unix.go.
const (
	tcget = unix.TCGETS
	tcset = unix.TCSETS
)
