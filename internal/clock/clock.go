// Package clock contains the application's core logic: a stored value (the
// current time) that is refreshed on a fixed interval and printed via a
// printer.Printer. Renderers that want to display it (e.g. the tview UI)
// poll Now() on their own schedule instead of being pushed updates.
package clock

import (
	"context"
	"sync"
	"time"

	"test/internal/printer"
)

// Format is the layout used to render the time everywhere in the app.
const Format = time.RFC1123

// Clock holds the current time, refreshed on a fixed interval.
type Clock struct {
	interval time.Duration

	mu  sync.RWMutex
	now time.Time
}

// New returns a Clock holding the current time, ticking every interval once
// Run is called.
func New(interval time.Duration) *Clock {
	return &Clock{interval: interval, now: time.Now()}
}

// Now returns the most recently stored time.
func (c *Clock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

// Run is the main logic loop: on every tick it refreshes the stored time
// and prints it via p, until ctx is cancelled.
func (c *Clock) Run(ctx context.Context, p *printer.Printer) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.update(p)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.update(p)
		}
	}
}

func (c *Clock) update(p *printer.Printer) {
	c.mu.Lock()
	c.now = time.Now()
	c.mu.Unlock()
	p.Println(c.Now().Format(Format))
}
