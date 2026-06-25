package pac

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"test/internal/service/printer"
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

func newTestPac(t *testing.T, pacJs string) *PacExecutor {
	t.Helper()
	pe, err := NewPac(pacJs, printer.New())
	if err != nil {
		t.Fatalf("NewPac: %v", err)
	}
	return pe
}

func TestNewPacCompilesValidScript(t *testing.T) {
	if _, err := NewPac(`function FindProxyForURL(url, host) { return "DIRECT"; }`, printer.New()); err != nil {
		t.Fatalf("NewPac: %v", err)
	}
}

func TestNewPacRejectsInvalidScript(t *testing.T) {
	if _, err := NewPac(`function FindProxyForURL(url, host) {`, printer.New()); err == nil {
		t.Fatal("NewPac: expected error for invalid script, got nil")
	}
}

func TestRunReturnsScriptDecision(t *testing.T) {
	pe := newTestPac(t, `function FindProxyForURL(url, host) { return "DIRECT"; }`)
	got, err := pe.Run("http://example.com", "example.com")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "DIRECT" {
		t.Fatalf("Run = %q, want %q", got, "DIRECT")
	}
}

func TestRunPassesUrlAndHost(t *testing.T) {
	pe := newTestPac(t, `function FindProxyForURL(url, host) { return url + "|" + host; }`)
	got, err := pe.Run("http://example.com/path", "example.com")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := "http://example.com/path|example.com"
	if got != want {
		t.Fatalf("Run = %q, want %q", got, want)
	}
}

func TestRunSurfacesScriptRuntimeError(t *testing.T) {
	pe := newTestPac(t, `function FindProxyForURL(url, host) { return undefinedVariable; }`)
	if _, err := pe.Run("http://example.com", "example.com"); err == nil {
		t.Fatal("Run: expected error for undefined variable, got nil")
	}
}

func TestAlertLogsThroughInjectedPrinter(t *testing.T) {
	p := printer.New()
	pe, err := NewPac(`function FindProxyForURL(url, host) { alert("hi from pac"); return "DIRECT"; }`, p)
	if err != nil {
		t.Fatalf("NewPac: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	out := captureStdout(t, func() {
		go func() {
			p.Run(ctx)
			close(done)
		}()
		if _, err := pe.Run("http://example.com", "example.com"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if err := p.Flush(ctx); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		cancel()
		<-done
	})

	if !strings.Contains(out, "hi from pac") {
		t.Fatalf("expected output to contain %q, got %q", "hi from pac", out)
	}
}

func TestRunDoesNotLeakStateBetweenPooledRuntimes(t *testing.T) {
	pe := newTestPac(t, `function FindProxyForURL(url, host) { return url + "|" + host; }`)

	got1, err := pe.Run("http://first.example.com", "first.example.com")
	if err != nil {
		t.Fatalf("Run (first): %v", err)
	}
	if want := "http://first.example.com|first.example.com"; got1 != want {
		t.Fatalf("Run (first) = %q, want %q", got1, want)
	}

	got2, err := pe.Run("http://second.example.com", "second.example.com")
	if err != nil {
		t.Fatalf("Run (second): %v", err)
	}
	if want := "http://second.example.com|second.example.com"; got2 != want {
		t.Fatalf("Run (second) = %q, want %q", got2, want)
	}
}
