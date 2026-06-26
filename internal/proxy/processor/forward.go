package processor

import (
	"crypto/tls"
	"net"

	"test/internal/config"
	"test/internal/proxy/message"
)

// connectForward establishes a connection to an upstream HTTP proxy (axis A =
// http-forward) for the kerberos/basic/anonymous types. The authenticator
// (axis B) supplies the Proxy-Authorization separately, and the request is
// always spoken to the proxy (absolute URL or CONNECT), so no CONNECT is
// simulated. Without the connection pool (§5) this dials fresh every time.
func (p *Process) connectForward(client *message.ProxyRequest, rule *config.Rule, firstProxy *config.Proxy, firstHostPort string, dialer *net.Dialer) (net.Conn, bool, error) {
	var conn net.Conn
	var err error
	if firstProxy.Ssl {
		conn, err = tls.DialWithDialer(dialer, "tcp4", firstHostPort, &tls.Config{})
	} else {
		conn, err = dialer.Dial("tcp4", firstHostPort)
	}
	return conn, false, err
}
