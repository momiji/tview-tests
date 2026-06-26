package processor

import (
	"sync/atomic"
	"time"
)

// TrafficRow records per-connection byte counters and timestamps for the UI.
// It implements transport.TrafficMeter. This is a minimal stand-in until the
// traffic UI is reconciled (MIGRATION.md step 14); the full rate-tracking row
// and its table live in the UI.
type TrafficRow struct {
	ReqId         int32
	Url           string
	BytesSent     atomic.Int64
	BytesReceived atomic.Int64
	lastSend      atomic.Int64 // unix nanos
	lastReceive   atomic.Int64
}

func NewTrafficRow(reqId int32, url string) *TrafficRow {
	r := &TrafficRow{ReqId: reqId, Url: url}
	now := time.Now().UnixNano()
	r.lastSend.Store(now)
	r.lastReceive.Store(now)
	return r
}

func (r *TrafficRow) AddReceived(n int) {
	r.BytesReceived.Add(int64(n))
	r.lastReceive.Store(time.Now().UnixNano())
}

func (r *TrafficRow) AddSent(n int) {
	r.BytesSent.Add(int64(n))
	r.lastSend.Store(time.Now().UnixNano())
}
