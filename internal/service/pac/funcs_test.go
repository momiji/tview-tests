package pac

import (
	"testing"
	"time"
)

func TestIsPlainHostName(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"www", true},
		{"www.example.com", false},
	}
	for _, c := range cases {
		if got := isPlainHostName(c.host); got != c.want {
			t.Errorf("isPlainHostName(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestDnsDomainIs(t *testing.T) {
	cases := []struct {
		host, domain string
		want         bool
	}{
		{"www.example.com", "example.com", true},
		{"example.com", "example.com", true},
		{"www.example.com", "other.com", false},
		{"notexample.com", "example.com", false},
	}
	for _, c := range cases {
		if got := dnsDomainIs(c.host, c.domain); got != c.want {
			t.Errorf("dnsDomainIs(%q, %q) = %v, want %v", c.host, c.domain, got, c.want)
		}
	}
}

func TestLocalHostOrDomainIs(t *testing.T) {
	cases := []struct {
		host, hostdom string
		want          bool
	}{
		{"www", "www.example.com", true},
		{"www.example.com", "www.example.com", true},
		{"www.example.com", "www.other.com", false},
		{"other", "www.example.com", false},
	}
	for _, c := range cases {
		if got := localHostOrDomainIs(c.host, c.hostdom); got != c.want {
			t.Errorf("localHostOrDomainIs(%q, %q) = %v, want %v", c.host, c.hostdom, got, c.want)
		}
	}
}

func TestConvertAddr(t *testing.T) {
	cases := []struct {
		ip   string
		want int64
	}{
		{"0.0.0.0", 0},
		{"255.255.255.0", 0xFFFFFF00},
		{"127.0.0.1", 0x7F000001},
		{"not-an-ip", 0},
		{"::1", 0}, // IPv6 unsupported, returns 0 rather than a misleading value
	}
	for _, c := range cases {
		if got := convert_addr(c.ip); got != c.want {
			t.Errorf("convert_addr(%q) = %#x, want %#x", c.ip, got, c.want)
		}
	}
}

func TestDnsDomainLevels(t *testing.T) {
	cases := []struct {
		host string
		want int
	}{
		{"www", 0},
		{"example.com", 1},
		{"www.example.com", 2},
		{"example.com.", 1}, // trailing dot shouldn't count as an extra level
	}
	for _, c := range cases {
		if got := dnsDomainLevels(c.host); got != c.want {
			t.Errorf("dnsDomainLevels(%q) = %d, want %d", c.host, got, c.want)
		}
	}
}

func TestShExpMatch(t *testing.T) {
	cases := []struct {
		str, shexp string
		want       bool
	}{
		{"www.example.com", "*.example.com", true},
		{"example.com", "*.example.com", false},
		{"http://example.com/path", "*/path", true},
		{"foo.txt", "*.doc", false},
	}
	for _, c := range cases {
		if got := shExpMatch(c.str, c.shexp); got != c.want {
			t.Errorf("shExpMatch(%q, %q) = %v, want %v", c.str, c.shexp, got, c.want)
		}
	}
}

func TestWeekdayRangeSingleDayForm(t *testing.T) {
	today := days[int(time.Now().Weekday())]
	otherDay := days[(int(time.Now().Weekday())+1)%7]

	if !weekdayRange(today, "", "") {
		t.Errorf("weekdayRange(%q, \"\", \"\") = false, want true", today)
	}
	if weekdayRange(otherDay, "", "") {
		t.Errorf("weekdayRange(%q, \"\", \"\") = true, want false", otherDay)
	}
}

func TestWeekdayRangeSingleDayGmtForm(t *testing.T) {
	today := days[int(time.Now().UTC().Weekday())]
	otherDay := days[(int(time.Now().UTC().Weekday())+1)%7]

	if !weekdayRange(today, "GMT", "") {
		t.Errorf("weekdayRange(%q, \"GMT\", \"\") = false, want true", today)
	}
	if weekdayRange(otherDay, "GMT", "") {
		t.Errorf("weekdayRange(%q, \"GMT\", \"\") = true, want false", otherDay)
	}
}

func TestWeekdayRangeUnknownDayIsFalse(t *testing.T) {
	if weekdayRange("NOTADAY", "", "") {
		t.Error(`weekdayRange("NOTADAY", "", "") = true, want false`)
	}
}

func TestDateRangeAndTimeRangeAreStubs(t *testing.T) {
	if !dateRange() {
		t.Error("dateRange() = false, want true (stub always returns true)")
	}
	if !timeRange() {
		t.Error("timeRange() = false, want true (stub always returns true)")
	}
}
