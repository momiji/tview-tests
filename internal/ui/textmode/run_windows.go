//go:build windows

package textmode

import (
	"context"
	"os"
	"sync"

	"golang.org/x/term"
)

var (
	readerOnce sync.Once
	keys       chan byte
)

// startReader lazily starts a single goroutine that reads one byte at a
// time from stdin for the rest of the process's life, publishing each one
// to keys. Unlike the poll-based unix implementation (run_unix.go),
// Windows has no equivalent of poll() on stdin to bound a blocking Read
// with a timeout, so a blocked Read can't be abandoned cleanly when Run
// returns. Sharing a single, persistent reader across every call to Run
// avoids ever having two goroutines racing to Read the same keypress.
func startReader() chan byte {
	readerOnce.Do(func() {
		keys = make(chan byte, 16)
		go func() {
			buf := make([]byte, 1)
			for {
				n, err := os.Stdin.Read(buf)
				if err != nil || n == 0 {
					close(keys)
					return
				}
				keys <- buf[0]
			}
		}()
	})
	return keys
}

// Run puts stdin into raw mode and waits for a key, switchSignal, or ctx —
// space and a closed switchSignal return SwitchToUI; q/Q/Ctrl-C and a
// cancelled ctx return Quit. Pass nil for switchSignal to only react to
// keypresses.
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

	keys := startReader()
	for {
		select {
		case <-ctx.Done():
			return Quit, nil
		case <-switchSignal:
			return SwitchToUI, nil
		case b, ok := <-keys:
			if !ok {
				return Quit, nil
			}
			switch b {
			case ' ':
				return SwitchToUI, nil
			case 'q', 'Q', 0x03: // 0x03 is Ctrl-C, which raw mode no longer raises as a signal.
				return Quit, nil
			}
		}
	}
}
