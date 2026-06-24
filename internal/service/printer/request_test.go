package printer

import (
	"strings"
	"testing"
)

func TestReqInfofTagsLineWithIdAndName(t *testing.T) {
	ti := NewReqLogInfo(42, "process")
	out := runWithPrinter(t, func(p *Printer) {
		p.ReqInfof(ti, "start connection (retryable=%d)", 1)
	})
	if !strings.Contains(out, "(42) process: start connection (retryable=1)") {
		t.Fatalf("expected tagged line, got %q", out)
	}
}

func TestReqHeaderfRedactsProxyAuthorization(t *testing.T) {
	header := "Proxy-Authorization: Basic " + strings.Repeat("a", 80)
	out := runWithPrinter(t, func(p *Printer) {
		p.ReqHeaderf("%s %s", ">", header)
	})
	if strings.Contains(out, strings.Repeat("a", 80)) {
		t.Fatalf("expected long proxy-authorization value to be redacted, got %q", out)
	}
	if !strings.Contains(out, "...") {
		t.Fatalf("expected redacted header to be truncated with '...', got %q", out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out[strings.Index(out, ">"):]), "> Proxy-Authorization") {
		t.Fatalf("expected prefix and header name to survive redaction, got %q", out)
	}
}

func TestReqHeaderfLeavesOtherHeadersUntouched(t *testing.T) {
	out := runWithPrinter(t, func(p *Printer) {
		p.ReqHeaderf("%s %s", ">", "Content-Type: text/plain")
	})
	if !strings.Contains(out, "> Content-Type: text/plain") {
		t.Fatalf("expected non-proxy-authorization header to pass through unchanged, got %q", out)
	}
}

func TestReqHeaderfKeepsShortProxyAuthorizationReadable(t *testing.T) {
	header := "Proxy-Authorization: Basic abc"
	out := runWithPrinter(t, func(p *Printer) {
		p.ReqHeaderf("%s %s", ">", header)
	})
	if !strings.Contains(out, "Proxy-Authorization") {
		t.Fatalf("expected header name to survive redaction, got %q", out)
	}
}
