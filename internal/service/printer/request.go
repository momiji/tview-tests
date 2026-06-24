package printer

import (
	"fmt"
	"strings"
)

// traceInfo tags a connection/request for tracing: a request id plus a
// short name identifying which stage of the pipeline is logging.
type traceInfo struct {
	reqId int32
	name  string
}

func newTraceInfo(reqId int32, name string) *traceInfo {
	return &traceInfo{reqId, name}
}

// Tracef logs a per-request trace line tagged with ti's id and name.
func (p *Printer) Tracef(ti *traceInfo, format string, args ...interface{}) {
	p.Printf("(%d) %s: %s", ti.reqId, ti.name, fmt.Sprintf(format, args...))
}

// Header logs an HTTP header, redacting the value of Proxy-Authorization
// down to a short prefix instead of printing it in full.
func (p *Printer) Header(format string, prefix string, header string) {
	lower := strings.ToLower(header)
	if strings.HasPrefix(lower, "proxy-authorization:") {
		l := len(header)
		if l-10 > 50 {
			l = 50
		} else {
			l = l - 10
			if l < 20 {
				l = 20
			}
		}
		header = header[:l] + "..."
	}
	p.Infof(format, prefix, header)
}
