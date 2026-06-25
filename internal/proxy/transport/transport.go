// Package transport holds the low-level connection helpers shared by the
// proxy: TCP tuning, a byte-counting connection wrapper, and an
// instrumented HTTP "chunked" reader/writer.
package transport

import (
	"crypto/tls"
	"net"
	"time"
)

// ConfigureConn reduces TIME_WAIT connections by disabling Nagle's algorithm
// on the underlying TCP connection.
func ConfigureConn(conn net.Conn) {
	if c, ok := conn.(*net.TCPConn); ok {
		_ = c.SetNoDelay(true)
		return
	}
	if c, ok := conn.(*tls.Conn); ok {
		ConfigureConn(c.NetConn())
		return
	}
}

// TrafficMeter records the bytes flowing through a TrafficConn. It is the
// port through which a UI's per-connection traffic row is updated, so this
// package does not depend on the UI. A nil meter disables accounting.
type TrafficMeter interface {
	AddReceived(n int)
	AddSent(n int)
}

// TrafficConn wraps a net.Conn and reports read/written byte counts to an
// optional TrafficMeter.
type TrafficConn struct {
	conn  net.Conn
	meter TrafficMeter
}

func NewTrafficConn(conn net.Conn) *TrafficConn {
	return &TrafficConn{conn: conn}
}

// SetMeter attaches (or clears, when nil) the meter receiving byte counts.
func (c *TrafficConn) SetMeter(m TrafficMeter) {
	c.meter = m
}

func (c *TrafficConn) Read(b []byte) (n int, err error) {
	n, err = c.conn.Read(b)
	if c.meter != nil {
		c.meter.AddReceived(n)
	}
	return
}

func (c *TrafficConn) Write(b []byte) (n int, err error) {
	n, err = c.conn.Write(b)
	if c.meter != nil {
		c.meter.AddSent(n)
	}
	return
}

func (c *TrafficConn) Close() error {
	return c.conn.Close()
}

func (c *TrafficConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *TrafficConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *TrafficConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *TrafficConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *TrafficConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
