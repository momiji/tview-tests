// Package traffic holds the per-connection traffic model shown by the UI: a
// row of byte-rate counters per request, and a table of rows. A TrafficRow
// implements transport.TrafficMeter, so the proxy updates it directly while
// forwarding, and Sink adapts the table to the processor's TrafficSink port
// so the processor stays UI-agnostic.
package traffic

import (
	"slices"
	"sync"
	"time"

	ratecounter "github.com/enterprizesoftware/rate-counter"

	"test/internal/proxy/transport"
)

type TrafficRow struct {
	ReqId                  int32
	Url                    string
	BytesSentPerSecond     *ratecounter.Rate
	BytesReceivedPerSecond *ratecounter.Rate
	Removed                time.Time
	LastSend               time.Time
	LastReceive            time.Time
}

func NewTrafficRow(reqId int32, url string) *TrafficRow {
	return &TrafficRow{
		ReqId:                  reqId,
		Url:                    url,
		BytesSentPerSecond:     ratecounter.New(100*time.Millisecond, 5*time.Second),
		BytesReceivedPerSecond: ratecounter.New(100*time.Millisecond, 5*time.Second),
		LastSend:               time.Now(),
		LastReceive:            time.Now(),
	}
}

// AddReceived and AddSent implement transport.TrafficMeter.
func (r *TrafficRow) AddReceived(n int) {
	r.BytesReceivedPerSecond.IncrementBy(n)
	r.LastReceive = time.Now()
}

func (r *TrafficRow) AddSent(n int) {
	r.BytesSentPerSecond.IncrementBy(n)
	r.LastSend = time.Now()
}

type TrafficTable struct {
	table []*TrafficRow
	lock  sync.RWMutex
}

func NewTrafficTable() *TrafficTable {
	return &TrafficTable{table: make([]*TrafficRow, 0)}
}

func (t *TrafficTable) Add(row *TrafficRow) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.table = append(t.table, row)
}

func (t *TrafficTable) Count() int {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return len(t.table)
}

func (t *TrafficTable) Get(pos int) *TrafficRow {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return t.table[pos]
}

func (t *TrafficTable) RowsCopy() []*TrafficRow {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return slices.Clone(t.table)
}

// Remove marks a row for deletion; RemoveDead later drops marked rows.
func (t *TrafficTable) Remove(row *TrafficRow) {
	row.Removed = time.Now()
}

func (t *TrafficTable) RemoveDead() {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.table = slices.DeleteFunc(t.table, func(row *TrafficRow) bool {
		return !row.Removed.IsZero() && time.Since(row.Removed) > 30*time.Second
	})
}

// Sink adapts a TrafficTable to the processor's TrafficSink port: it creates a
// metered row per request and retires it on close.
type Sink struct {
	table *TrafficTable
}

func NewSink(table *TrafficTable) *Sink {
	return &Sink{table: table}
}

func (s *Sink) New(reqId int32, url string) transport.TrafficMeter {
	row := NewTrafficRow(reqId, url)
	s.table.Add(row)
	return row
}

func (s *Sink) Remove(m transport.TrafficMeter) {
	if row, ok := m.(*TrafficRow); ok {
		s.table.Remove(row)
	}
}
