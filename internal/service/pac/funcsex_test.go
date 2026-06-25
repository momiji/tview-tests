package pac

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestIsInNetEx(t *testing.T) {
	cases := []struct {
		name      string
		ipAddress string
		ipPrefix  string
		want      bool
	}{
		{"ipv4 inside prefix", "192.168.1.42", "192.168.1.0/24", true},
		{"ipv4 outside prefix", "192.168.2.42", "192.168.1.0/24", false},
		{"ipv6 inside prefix", "2001:db8::1234", "2001:db8::/32", true},
		{"ipv6 outside prefix", "2001:db9::1234", "2001:db8::/32", false},
		{"invalid prefix", "192.168.1.42", "not-a-cidr", false},
		{"invalid address", "not-an-ip", "192.168.1.0/24", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isInNetEx(c.ipAddress, c.ipPrefix); got != c.want {
				t.Errorf("isInNetEx(%q, %q) = %v, want %v", c.ipAddress, c.ipPrefix, got, c.want)
			}
		})
	}
}

func TestMyIpAddressEx(t *testing.T) {
	got := myIpAddressEx()
	if got == "" {
		t.Fatal("myIpAddressEx() returned empty string")
	}
	for _, ip := range strings.Split(got, ";") {
		if net.ParseIP(ip) == nil {
			t.Errorf("myIpAddressEx() returned invalid IP %q in %q", ip, got)
		}
	}
}

func TestDnsResolveExAndIsResolvableEx(t *testing.T) {
	const timeout = 2 * time.Second

	if !isResolvableEx("localhost", timeout) {
		t.Error(`isResolvableEx("localhost", timeout) = false, want true`)
	}
	resolved := dnsResolveEx("localhost", timeout)
	if resolved == "" {
		t.Fatal(`dnsResolveEx("localhost", timeout) returned empty string`)
	}
	for _, ip := range strings.Split(resolved, ";") {
		if net.ParseIP(ip) == nil {
			t.Errorf("dnsResolveEx(\"localhost\", timeout) returned invalid IP %q in %q", ip, resolved)
		}
	}

	const badHost = "this-host-does-not-exist.invalid"
	if isResolvableEx(badHost, timeout) {
		t.Errorf("isResolvableEx(%q, timeout) = true, want false", badHost)
	}
	if got := dnsResolveEx(badHost, timeout); got != "" {
		t.Errorf("dnsResolveEx(%q, timeout) = %q, want \"\"", badHost, got)
	}
}
