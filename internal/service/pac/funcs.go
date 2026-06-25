package pac

import (
	"context"
	"math/big"
	"net"
	"regexp"
	"strings"
	"time"
)

// https://developer.mozilla.org/en-US/docs/Web/HTTP/Proxy_servers_and_tunneling/Proxy_Auto-Configuration_(PAC)_file#isPlainHostName

func isPlainHostName(host string) bool {
	return !strings.Contains(host, ".")
}
func dnsDomainIs(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}
func localHostOrDomainIs(host, hostdom string) bool {
	return host == hostdom || (!strings.Contains(host, ".") && strings.HasPrefix(hostdom, host+"."))
}
func isResolvable(host string, timeout time.Duration) bool {
	_, err := lookupHost(host, timeout)
	return err == nil
}
func isInNet(host, pattern, mask string, timeout time.Duration) bool {
	host = dnsResolve(host, timeout)
	if host == "" {
		return false
	}
	hostInt := convert_addr(host)
	patternInt := convert_addr(pattern)
	maskInt := convert_addr(mask)
	return hostInt&maskInt == patternInt
}
func dnsResolve(host string, timeout time.Duration) string {
	ips, err := lookupHost(host, timeout)
	if err != nil || len(ips) == 0 {
		return ""
	}
	return ips[0]
}

// lookupHost bounds net.DefaultResolver.LookupHost with timeout, so a slow
// or unresponsive DNS server can't hang a PAC script's evaluation forever.
func lookupHost(host string, timeout time.Duration) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return net.DefaultResolver.LookupHost(ctx, host)
}

// convert_addr only supports IPv4, matching the rest of the classic PAC
// builtins (isInNet's pattern/mask are always dotted-quad). ip.To4()
// returns nil for an IPv6 address, in which case this returns 0 rather
// than a value that would look like a plausible (but wrong) match.
func convert_addr(ipaddr string) int64 {
	ip := net.ParseIP(ipaddr)
	if ip == nil {
		return 0
	}
	v4 := ip.To4()
	if v4 == nil {
		return 0
	}
	ipInt := big.NewInt(0)
	ipInt.SetBytes(v4)
	return ipInt.Int64()
}
func myIpAddress() string {
	// Fallback if no suitable route/interface is found.
	ip := "127.0.0.1"

	// UDP "dial" does not send packets here; it just asks kernel for route/source IP.
	conn, err := net.Dial("udp4", "1.1.1.1:80")
	if err == nil {
		defer conn.Close()
		if ua, ok := conn.LocalAddr().(*net.UDPAddr); ok && ua.IP != nil && !ua.IP.IsLoopback() {
			return ua.IP.String()
		}
	}
	return ip
}
func dnsDomainLevels(host string) int {
	host = strings.TrimSuffix(host, ".")
	return len(strings.Split(host, ".")) - 1
}
func shExpMatch(str, shexp string) bool {
	shexp = strings.ReplaceAll(shexp, ".", `\.`)
	shexp = strings.ReplaceAll(shexp, "*", ".*")
	shexp = strings.ReplaceAll(shexp, "?", ".")
	shexp = "^" + shexp + "$"
	regex, err := regexp.Compile(shexp)
	if err != nil {
		// Malformed shell-expression pattern: no match, rather than
		// relying on a nil *Regexp's behavior implicitly.
		return false
	}
	return regex.MatchString(str)
}

var days = [...]string{"SUN", "MON", "TUE", "WEN", "THU", "FRI", "SAT"}

func dayIndex(day string) int {
	for i, d := range days {
		if d == day {
			return i
		}
	}
	return -1
}

// weekdayRange supports all three PAC call forms: a single day
// ("MON"), a single day with GMT ("MON", "GMT"), and a day range
// (start, end[, "GMT"]).
func weekdayRange(start, end, tz string) bool {
	startDay := dayIndex(start)
	endDay := dayIndex(end)
	switch end {
	case "GMT":
		tz = "GMT"
		endDay = startDay
	case "":
		// 1-arg form: a single day, no range.
		endDay = startDay
	}
	if startDay < 0 || endDay < 0 {
		return false
	}
	today := time.Now()
	if tz == "GMT" {
		today = today.UTC()
	}
	weekDay := int(today.Weekday())
	if startDay <= weekDay && weekDay <= endDay {
		return true
	}
	weekDay += 7
	if startDay <= weekDay && weekDay <= endDay {
		return true
	}
	return false
}
func dateRange() bool {
	// TODO implement PAC dateRange()
	return true
}
func timeRange() bool {
	// TODO implement PAC timeRange()
	return true
}
