// Package tui implements the tview-based UI mode: a full-screen view of
// the current time, with spacebar and q/Q/Ctrl-C handled as mode-switch
// and quit signals respectively.
package tui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"test/internal/clock"
)

// Signal is the reason Run returned.
type Signal int

const (
	// SwitchToText means the user pressed space and wants the plain console.
	SwitchToText Signal = iota
	// Quit means the user pressed q/Q/Ctrl-C.
	Quit
)

// refreshInterval is how often the UI polls the clock's stored value,
// independent of the clock's own tick interval.
const refreshInterval = 500 * time.Millisecond

// Run renders the tview UI until the user presses space (SwitchToText) or
// q/Q/Ctrl-C (Quit). It owns the terminal screen for the duration of the
// call and always restores it (tview's Application.Stop tears down the
// screen) before returning, so the caller can safely fall back to plain
// console mode.
func Run(clk *clock.Clock) (Signal, error) {
	textView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter)
	textView.SetBorder(true).
		SetTitle(" tview-tests — space: text mode · q/Ctrl-C: quit ")
	textView.SetText(fmt.Sprintf("\n%s", clk.Now().Format(clock.Format)))

	app := tview.NewApplication().SetRoot(textView, true)

	signal := Quit
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Rune() == ' ':
			signal = SwitchToText
			app.Stop()
			return nil
		case event.Key() == tcell.KeyCtrlC, event.Rune() == 'q', event.Rune() == 'Q':
			signal = Quit
			app.Stop()
			return nil
		}
		return event
	})

	done := make(chan struct{})
	defer close(done)

	go func() {
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				now := clk.Now()
				app.QueueUpdateDraw(func() {
					textView.SetText(fmt.Sprintf("\n%s", now.Format(clock.Format)))
				})
			}
		}
	}()

	if err := app.Run(); err != nil {
		return Quit, err
	}
	return signal, nil
}
