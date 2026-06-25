// Package printer provides an asynchronous, toggleable stdout writer.
// Println enqueues a line and returns immediately; a background worker
// (Run) does the actual writing, so callers never block on I/O regardless
// of which UI mode is active. This lets the same clock logic feed both
// plain-console output and a tview UI without the caller needing to know
// which mode is active or worry about stdout latency.
package printer

import (
	"context"
	"fmt"
	"sync"
)

// queueSize bounds how many not-yet-written lines can be buffered. Once
// full, Println drops the line rather than blocking the caller. Sized for
// bursts of per-request trace logging, not just the clock's one-line-every-
// 2s ticks: 10000 is tuned for that proxy-traffic volume.
const queueSize = 10000

// job is either a line to write, a flush request (ack set, line empty), or
// both unset (never produced, but harmless if it were).
type job struct {
	line string
	ack  chan struct{}
}

// Printer is a toggleable, asynchronous stdout writer. Call Run once to
// start the background worker that actually writes queued lines.
type Printer struct {
	mu      sync.Mutex
	enabled bool
	queue   chan job
}

// New returns a Printer that starts enabled.
func New() *Printer {
	return &Printer{enabled: true, queue: make(chan job, queueSize)}
}

// Enable makes subsequent Println calls queue lines for writing.
func (p *Printer) Enable() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = true
}

// Disable suppresses output: subsequent Println calls won't queue anything
// new, and any lines already queued (from before Disable was called) are
// discarded by the worker instead of being written, so disabling always
// takes effect immediately rather than after a backlog drains. Because
// write() checks p.enabled under the same lock it holds while actually
// printing, no line can be printed once this call returns: any write()
// already mid-print finishes (and was already committed to printing)
// before this can acquire the lock, and any write() that acquires the
// lock afterwards is guaranteed to see enabled == false.
func (p *Printer) Disable() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = false
}

// Println queues a line to be written to stdout, unless the printer is
// disabled. It never blocks on I/O: if the queue is full, the line is
// dropped rather than stalling the caller.
func (p *Printer) Println(a ...any) {
	p.mu.Lock()
	enabled := p.enabled
	p.mu.Unlock()
	if !enabled {
		return
	}
	select {
	case p.queue <- job{line: fmt.Sprintln(a...)}:
	default:
	}
}

// Run is the background worker: it writes queued lines to stdout, in
// order, until ctx is cancelled, then drains whatever is already queued
// before returning.
func (p *Printer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			p.drain()
			return
		case j := <-p.queue:
			p.write(j)
		}
	}
}

func (p *Printer) drain() {
	for {
		select {
		case j := <-p.queue:
			p.write(j)
		default:
			return
		}
	}
}

// write prints j's line, unless the printer is disabled. The check and the
// print happen under the same lock Disable() takes, so that once Disable()
// returns no line can be printed after it — see the comment on Disable.
func (p *Printer) write(j job) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enabled && j.line != "" {
		fmt.Print(j.line)
	}
	if j.ack != nil {
		close(j.ack)
	}
}

// Flush blocks until every line queued before this call has been
// processed (written, or discarded if the printer was disabled in the
// meantime), or ctx is cancelled first. It works by queuing a marker after
// them and waiting for the worker to reach it, so it relies on Run having
// been started.
func (p *Printer) Flush(ctx context.Context) error {
	ack := make(chan struct{})
	select {
	case p.queue <- job{ack: ack}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-ack:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
