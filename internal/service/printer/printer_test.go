package printer

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	return string(out)
}

func TestPrintlnWritesViaRun(t *testing.T) {
	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	out := captureStdout(t, func() {
		go func() {
			p.Run(ctx)
			close(done)
		}()
		p.Println("hello")
		if err := p.Flush(ctx); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		cancel()
		<-done
	})

	if !strings.Contains(out, "hello") {
		t.Fatalf("expected output to contain %q, got %q", "hello", out)
	}
}

func TestDisablePreventsOutput(t *testing.T) {
	p := New()
	p.Disable()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	out := captureStdout(t, func() {
		go func() {
			p.Run(ctx)
			close(done)
		}()
		p.Println("should not appear")
		if err := p.Flush(ctx); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		cancel()
		<-done
	})

	if out != "" {
		t.Fatalf("expected no output while disabled, got %q", out)
	}
}

func TestEnableAfterDisableResumesOutput(t *testing.T) {
	p := New()
	p.Disable()
	p.Enable()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	out := captureStdout(t, func() {
		go func() {
			p.Run(ctx)
			close(done)
		}()
		p.Println("visible again")
		if err := p.Flush(ctx); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		cancel()
		<-done
	})

	if !strings.Contains(out, "visible again") {
		t.Fatalf("expected output to contain %q, got %q", "visible again", out)
	}
}

func TestDisableDiscardsAlreadyQueuedLines(t *testing.T) {
	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	out := captureStdout(t, func() {
		// Queue a line while enabled, but don't start Run yet, then
		// disable before the worker ever sees it.
		p.Println("queued before disable")
		p.Disable()

		go func() {
			p.Run(ctx)
			close(done)
		}()
		if err := p.Flush(ctx); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		cancel()
		<-done
	})

	if out != "" {
		t.Fatalf("expected already-queued line to be discarded, got %q", out)
	}
}

func TestPrintlnDropsWhenQueueFull(t *testing.T) {
	p := New()
	// Don't start Run, so nothing drains the queue: fill it past capacity.
	for i := 0; i < queueSize+10; i++ {
		p.Println("line")
	}
	if len(p.queue) != queueSize {
		t.Fatalf("expected queue to be full at %d, got %d", queueSize, len(p.queue))
	}
}

func TestFlushReturnsAfterDrain(t *testing.T) {
	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	for i := 0; i < 5; i++ {
		p.Println("line", i)
	}
	if err := p.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	// queue should be drained by the time Flush returns.
	if len(p.queue) != 0 {
		t.Fatalf("expected queue to be drained after Flush, got len %d", len(p.queue))
	}
	cancel()
	<-done
}

func TestFlushReturnsErrorWhenContextCancelled(t *testing.T) {
	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled, Run never started: Flush can't enqueue or wait.

	if err := p.Flush(ctx); err == nil {
		t.Fatalf("expected Flush to return an error for a cancelled context")
	}
}

func TestRunDrainsQueueOnCancel(t *testing.T) {
	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	out := captureStdout(t, func() {
		p.Println("queued before run")
		cancel() // cancel before Run even starts
		go func() {
			p.Run(ctx)
			close(done)
		}()
		<-done
	})

	if !strings.Contains(out, "queued before run") {
		t.Fatalf("expected drain to write already-queued line, got %q", out)
	}
}

func TestConcurrentPrintlnAndDisableDoNotRace(t *testing.T) {
	captureStdout(t, func() {
		p := New()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			p.Run(ctx)
			close(done)
		}()

		stop := time.After(50 * time.Millisecond)
	loop:
		for {
			select {
			case <-stop:
				break loop
			default:
				p.Println("x")
				p.Disable()
				p.Enable()
			}
		}
		cancel()
		<-done
	})
}
