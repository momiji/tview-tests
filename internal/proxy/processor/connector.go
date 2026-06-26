package processor

import (
	"crypto/tls"
	"net"
	"strings"
	"time"

	"github.com/palantir/stacktrace"

	"test/internal/config"
	"test/internal/proxy/message"
)

// noAuth is the sentinel empty authorization (its pointer is shared, never
// mutated).
var noAuth = ""

func errNotMigrated(what string) error {
	return stacktrace.NewError("%s not yet migrated", what)
}

// connectUpstream establishes the upstream connection for firstProxy (axis A),
// returning the connection and whether a CONNECT must be simulated (for peers
// that do not speak HTTP, i.e. direct/socks tunnels).
func (p *Process) connectUpstream(client *message.ProxyRequest, firstProxy *config.Proxy, firstHostPort string, rule *config.Rule, authentication bool, authFunc func() (*string, error), authorization **string) (net.Conn, bool, error) {
	dialer := new(net.Dialer)
	dialer.Timeout = time.Duration(p.runtime.conf.ConnectTimeout) * time.Second
	switch *firstProxy.Type {
	case config.ProxyDirect:
		return p.connectDirect(client, rule, firstProxy, dialer)
	case config.ProxyKerberos, config.ProxyBasic, config.ProxyAnonymous:
		return p.connectForward(client, rule, firstProxy, firstHostPort, dialer)
	case config.ProxySocks:
		return p.connectSocks(client, rule, firstProxy, firstHostPort, dialer, authentication, authFunc, authorization)
	}
	return nil, false, errNotMigrated("unknown proxy type")
}

// connectDirect dials the origin server directly (axis A = direct).
func (p *Process) connectDirect(client *message.ProxyRequest, rule *config.Rule, firstProxy *config.Proxy, dialer *net.Dialer) (net.Conn, bool, error) {
	simulateConnect := client.Header.IsConnect
	hostPort := client.Header.HostPort
	host, port := splitHostPort(hostPort, "", "", false)
	if rule.Dns != nil {
		h2, p2 := splitHostPort(*rule.Dns, host, port, false)
		hostPort = h2 + ":" + p2
	}
	var conn net.Conn
	var err error
	if firstProxy.Ssl {
		conn, err = tls.DialWithDialer(dialer, "tcp4", hostPort, &tls.Config{})
	} else {
		conn, err = dialer.Dial("tcp4", hostPort)
	}
	return conn, simulateConnect, err
}

func proxyShortName(s string) string {
	if strings.Contains(s, ",") {
		return strings.Split(s, ",")[0] + "+"
	}
	return s
}

// splitHostPort splits "host:port" with defaults; when only one part is
// present, portFirst decides whether it is the host or the port.
func splitHostPort(hostPort, defaultHost, defaultPort string, portFirst bool) (string, string) {
	hp := strings.SplitN(hostPort, ":", 2)
	var host, port string
	if len(hp) == 1 {
		if portFirst {
			host = ""
			port = hp[0]
		} else {
			host = hp[0]
			port = ""
		}
	} else if len(hp) == 2 {
		host = hp[0]
		port = hp[1]
	}
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if host == "" {
		host = defaultHost
	}
	if port == "" {
		port = defaultPort
	}
	return host, port
}
