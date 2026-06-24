package printer

import (
	"context"
	"strings"
	"testing"
)

// runWithPrinter starts a Printer and its worker, runs fn, flushes, and
// returns everything written to stdout.
func runWithPrinter(t *testing.T, fn func(p *Printer)) string {
	t.Helper()
	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	out := captureStdout(t, func() {
		go func() {
			p.Run(ctx)
			close(done)
		}()
		fn(p)
		if err := p.Flush(ctx); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		cancel()
		<-done
	})
	return out
}

func TestPrintfPrefixesTimestamp(t *testing.T) {
	out := runWithPrinter(t, func(p *Printer) {
		p.Printf("hello %s", "world")
	})
	if !strings.Contains(out, "hello world") {
		t.Fatalf("expected output to contain %q, got %q", "hello world", out)
	}
	// timeFormat starts with a 4-digit year, e.g. "2026/06/25 ...".
	if len(out) < 4 || !strings.Contains(out[:5], "20") {
		t.Fatalf("expected output to start with a timestamp, got %q", out)
	}
}

func TestInfofFormatsArgs(t *testing.T) {
	out := runWithPrinter(t, func(p *Printer) {
		p.Infof("count=%d name=%s", 3, "abc")
	})
	if !strings.Contains(out, "count=3 name=abc") {
		t.Fatalf("expected formatted args, got %q", out)
	}
}

func TestErrorfFormatsArgs(t *testing.T) {
	out := runWithPrinter(t, func(p *Printer) {
		p.Errorf("boom: %v", "disk full")
	})
	if !strings.Contains(out, "boom: disk full") {
		t.Fatalf("expected formatted args, got %q", out)
	}
}
