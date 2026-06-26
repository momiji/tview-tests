package processor

import (
	"net"

	"test/internal/config"
	"test/internal/proxy/message"
)

// connectForward establishes a connection through an upstream HTTP proxy
// (axis A = http-forward) for the kerberos/basic/anonymous types. The
// authenticator (axis B) supplies the Proxy-Authorization separately.
//
// TODO(step 9b): port from process.go (the ProxyKerberos/Basic/Anonymous case
// of the connection switch), including the Ssl dial and the CONNECT/plain
// variants.
func (p *Process) connectForward(client *message.ProxyRequest, rule *config.Rule, firstProxy *config.Proxy, firstHostPort string, dialer *net.Dialer) (net.Conn, bool, error) {
	return nil, false, errNotMigrated("http-forward connector (step 9b)")
}
