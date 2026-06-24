// Package printer provides a stdout writer that can be toggled on and off.
// Callers keep calling Println unconditionally; the printer itself decides
// whether anything actually reaches the terminal. This lets the same clock
// logic feed both plain-console output and a tview UI without the caller
// needing to know which mode is active.
package printer

import (
	"fmt"
	"sync"
)

// Printer is a toggleable stdout writer.
type Printer struct {
	mu      sync.Mutex
	enabled bool
}

// New returns a Printer that starts enabled.
func New() *Printer {
	return &Printer{enabled: true}
}

// Enable makes subsequent Println calls write to stdout.
func (p *Printer) Enable() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = true
}

// Disable makes subsequent Println calls a no-op.
func (p *Printer) Disable() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = false
}

// Println writes to stdout, unless the printer is disabled.
func (p *Printer) Println(a ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enabled {
		fmt.Println(a...)
	}
}
