package processor

import (
	"net"

	"test/internal/config"
	"test/internal/proxy/message"
)

// connectSocks establishes a connection through an upstream SOCKS5 proxy
// (axis A = socks). When authenticated, it derives the SOCKS auth from the
// authorizationFunc.
//
// TODO(step 9d): port from process.go (the ProxySocks case of the connection
// switch), plus the SOCKS server entry point (processSocks / TCPHandle).
func (p *Process) connectSocks(client *message.ProxyRequest, rule *config.Rule, firstProxy *config.Proxy, firstHostPort string, dialer *net.Dialer, authentication bool, authFunc func() (*string, error), authorization **string) (net.Conn, bool, error) {
	return nil, false, errNotMigrated("socks connector (step 9d)")
}
