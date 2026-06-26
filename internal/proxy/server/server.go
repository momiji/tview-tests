// Package server runs the inbound listeners (HTTP proxy and SOCKS5),
// enforces the ACL, and hands each accepted connection to a processor. It
// shuts down on context cancellation.
package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"

	"github.com/palantir/stacktrace"
	"github.com/txthinking/socks5"

	"test/internal/config"
	"test/internal/proxy/processor"
	"test/internal/proxy/transport"
	"test/internal/service/printer"
)

// Server accepts client connections on the configured ports and dispatches
// them to processors built from the shared runtime.
type Server struct {
	runtime       *processor.Runtime
	conf          *config.ProxyConf
	printer       *printer.Printer
	requestsCount atomic.Int32
}

func New(runtime *processor.Runtime, conf *config.ProxyConf, p *printer.Printer) *Server {
	return &Server{runtime: runtime, conf: conf, printer: p}
}

// Run starts the HTTP and SOCKS5 listeners and blocks until ctx is cancelled
// or a listener fails.
func (s *Server) Run(ctx context.Context) error {
	errChan := make(chan error, 1)

	// start http server
	if s.conf.Port != 0 {
		ln, err := net.Listen("tcp4", fmt.Sprint(s.conf.Bind, ":", s.conf.Port))
		if err != nil {
			return stacktrace.Propagate(err, "unable to listen on %s:%d", s.conf.Bind, s.conf.Port)
		}
		hostPort := ln.Addr().String()
		s.printer.Infof("[-] Use %s as your http proxy or http://%s/proxy.pac as your proxy PAC url", hostPort, hostPort)
		go s.acceptHttp(ctx, ln)
	}

	// start socks5 server
	if s.conf.SocksPort != 0 {
		sks, err := socks5.NewClassicServer(fmt.Sprintf("%s:%d", s.conf.Bind, s.conf.SocksPort), s.conf.Bind, "", "", 0, 60)
		if err != nil {
			return stacktrace.Propagate(err, "unable to create socks server on %s:%d", s.conf.Bind, s.conf.SocksPort)
		}
		s.printer.Infof("[-] Use %s as your socks5 proxy and configure it to use remote dns - curl syntax is 'curl -x socks5h://%s' or 'curl --socks5-hostname %s'", sks.Addr, sks.Addr, sks.Addr)
		go func() {
			if err := sks.ListenAndServe(s); err != nil && ctx.Err() == nil {
				errChan <- stacktrace.Propagate(err, "unable to listen on %s:%d", s.conf.Bind, s.conf.SocksPort)
			}
		}()
	}

	// wait for shutdown or a fatal listener error
	select {
	case <-ctx.Done():
		return nil
	case err := <-errChan:
		return err
	}
}

func (s *Server) acceptHttp(ctx context.Context, ln net.Listener) {
	// close the listener on shutdown to unblock Accept
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		remoteIp := strings.Split(conn.RemoteAddr().String(), ":")[0]
		if !s.isAllowed(remoteIp, s.conf.ACL) {
			s.printer.Infof("[-] Connection from %s is not allowed by ACL", remoteIp)
			_ = conn.Close()
			continue
		}
		transport.ConfigureConn(conn)
		if ctx.Err() != nil {
			_ = conn.Close()
			return
		}
		go func() {
			s.requestsCount.Add(1)
			processor.NewProcess(s.runtime, conn).ProcessHttp()
			s.requestsCount.Add(-1)
		}()
	}
}

// TCPHandle implements the socks5 server handler: it replies success and runs
// a processor for the request.
func (s *Server) TCPHandle(server *socks5.Server, conn *net.TCPConn, request *socks5.Request) error {
	if request.Cmd != socks5.CmdConnect {
		s.printer.Infof("[-] TCP socks proxy is not implemented for command %b", request.Cmd)
		return nil
	}
	// return any address, not important as we are using connect
	a, addr, port, err := socks5.ParseAddress("127.0.0.1:12345")
	if err != nil {
		_ = conn.Close()
		return err
	}
	if a == socks5.ATYPDomain {
		addr = addr[1:]
	}
	reply := socks5.NewReply(socks5.RepSuccess, a, addr, port)
	if _, err := reply.WriteTo(conn); err != nil {
		_ = conn.Close()
		return err
	}
	processor.NewProcess(s.runtime, conn).ProcessSocks(request)
	return nil
}

func (s *Server) UDPHandle(server *socks5.Server, addr *net.UDPAddr, datagram *socks5.Datagram) error {
	s.printer.Infof("[-] UDP socks proxy is not implemented")
	return nil
}

// isAllowed reports whether ip is permitted by the ACL (a list of IPs or
// CIDRs). An empty ACL allows everybody.
func (s *Server) isAllowed(ip string, acl []string) bool {
	for _, a := range acl {
		if strings.Contains(a, "/") {
			_, cidr, _ := net.ParseCIDR(a)
			if cidr != nil && cidr.Contains(net.ParseIP(ip)) {
				return true
			}
		} else if a == ip {
			return true
		}
	}
	return len(acl) == 0
}
