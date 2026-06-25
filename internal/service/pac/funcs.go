package pac

import (
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
func isResolvable(host string) bool {
	_, err := net.LookupHost(host)
	return err == nil
}
func isInNet(host, pattern, mask string) bool {
	host = dnsResolve(host)
	if host == "" {
		return false
	}
	hostInt := convert_addr(host)
	patternInt := convert_addr(pattern)
	maskInt := convert_addr(mask)
	return hostInt&maskInt == patternInt
}
func dnsResolve(host string) string {
	ips, err := net.LookupHost(host)
	if err != nil {
		return ""
	}
	if len(ips) == 0 {
		return ""
	}
	return ips[0]
}
func convert_addr(ipaddr string) int64 {
	ip := net.ParseIP(ipaddr)
	if ip == nil {
		return 0
	}
	ipInt := big.NewInt(0)
	ipInt.SetBytes(ip.To4())
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
	return len(strings.Split(host, ".")) - 1
}
func shExpMatch(str, shexp string) bool {
	shexp = strings.ReplaceAll(shexp, ".", `\.`)
	shexp = strings.ReplaceAll(shexp, "*", ".*")
	shexp = strings.ReplaceAll(shexp, "?", ".")
	shexp = "^" + shexp + "$"
	regex, _ := regexp.Compile(shexp)
	return regex.MatchString(str)
}

var days = [...]string{"SUN", "MON", "TUE", "WEN", "THU", "FRI", "SAT"}

func weekdayRange(start, end, tz string) bool {
	startDay := -1
	endDay := -1
	for i, day := range days {
		if start == day {
			startDay = i
		}
		if end == day {
			endDay = i
		}
	}
	if end == "GMT" {
		tz = "GMT"
		endDay = startDay
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
