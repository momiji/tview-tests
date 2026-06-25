package pac

import (
	"net"
	"strings"
	"time"
)

// The "Ex" builtins are a Microsoft-originated PAC extension (supported by
// Firefox and Chrome) that adds IPv6 support alongside the classic,
// IPv4-only builtins in funcs.go: https://developer.mozilla.org/en-US/docs/Web/HTTP/Proxy_servers_and_tunneling/Proxy_Auto-Configuration_(PAC)_file

// dnsResolveEx resolves host to all of its IPs (v4 and v6), semicolon
// separated, or "" if resolution fails or times out.
func dnsResolveEx(host string, timeout time.Duration) string {
	ips, err := lookupHost(host, timeout)
	if err != nil || len(ips) == 0 {
		return ""
	}
	return strings.Join(ips, ";")
}

// isResolvableEx reports whether host resolves to at least one IP (v4 or
// v6) within timeout.
func isResolvableEx(host string, timeout time.Duration) bool {
	ips, err := lookupHost(host, timeout)
	return err == nil && len(ips) > 0
}

// isInNetEx reports whether ipAddress falls within ipPrefix, a CIDR (e.g.
// "2001:db8::/32" or "192.168.1.0/24"). Unlike isInNet's pattern/mask
// pair, CIDR notation works for both IPv4 and IPv6, since it doesn't rely
// on a fixed-width integer mask.
func isInNetEx(ipAddress, ipPrefix string) bool {
	_, ipNet, err := net.ParseCIDR(ipPrefix)
	if err != nil {
		return false
	}
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		return false
	}
	return ipNet.Contains(ip)
}

// myIpAddressEx returns every non-loopback IP (v4 and v6) configured on
// this host's interfaces, semicolon separated, or "127.0.0.1" if none are
// found. Unlike myIpAddress (which picks the single address the kernel
// would route outbound traffic from), this enumerates all of them per
// the Ex spec.
func myIpAddressEx() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	var ips []string
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		ips = append(ips, ipNet.IP.String())
	}
	if len(ips) == 0 {
		return "127.0.0.1"
	}
	return strings.Join(ips, ";")
}
