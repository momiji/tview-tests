package processor

import "test/internal/proxy/transport"

// TrafficSink creates a per-connection traffic meter and retires it when the
// connection closes. It is the port through which the UI's traffic table is
// fed, keeping this package independent of the UI. A nil sink (the default)
// disables traffic accounting.
type TrafficSink interface {
	New(reqId int32, url string) transport.TrafficMeter
	Remove(m transport.TrafficMeter)
}

// nopSink is the default sink: no accounting.
type nopSink struct{}

func (nopSink) New(int32, string) transport.TrafficMeter { return nil }
func (nopSink) Remove(transport.TrafficMeter)            {}
