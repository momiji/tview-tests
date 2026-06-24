package app

import (
	"fmt"

	"test/internal/textmode"
	"test/internal/tui"
)

// RunUI drives --ui mode on top of an already-started App (see Start). It
// starts in text mode and switches to the tview UI as soon as either the
// user presses space or autoSwitch fires — autoSwitch is owned and
// triggered by the caller, e.g. on a startup timer, so RunUI itself has no
// notion of why or when that happens. After the first switch, the user can
// keep toggling between text and UI mode with the spacebar. Pressing
// q/Q/Ctrl-C quits from either mode; because tview's Stop() and
// textmode's terminal restore both run before this function returns, the
// terminal is always left back in plain console state.
func RunUI(a *App, autoSwitch <-chan struct{}) error {
	fmt.Println("tview-tests --ui: starting in text mode, switching to UI mode automatically or on space...")

	sig, err := textmode.Run(autoSwitch)
	if err != nil {
		return err
	}
	if sig == textmode.Quit {
		return nil
	}

	for {
		a.Printer.Disable()
		uiSig, err := tui.Run(a.Clock)
		if err != nil {
			return err
		}
		if uiSig == tui.Quit {
			return nil
		}

		// User pressed space in the UI: back to text mode. No automatic
		// switch trigger here — only a keypress brings it back to the UI.
		a.Printer.Enable()
		textSig, err := textmode.Run(nil)
		if err != nil {
			return err
		}
		if textSig == textmode.Quit {
			return nil
		}
		// User pressed space in text mode: loop back into UI mode.
	}
}
