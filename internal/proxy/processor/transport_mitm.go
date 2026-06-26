package processor

import (
	"crypto/tls"
	"fmt"
	"io"

	"test/internal/config"
	"test/internal/proxy/message"
)

// transportMitm terminates TLS on both sides (axis C = mitm) and runs a
// decrypted request/response loop, so https flows can be inspected. It needs a
// certificate manager (the certs.Manager) to mint a leaf for the client side.
func (p *Process) transportMitm(clientChannel, proxyChannel *message.ProxyRequest, rule *config.Rule, firstProxy *config.Proxy, firstHostPort string, mitmClient, mitmProxy bool) *message.ProxyRequest {
	p.trace("mitm hijacking")
	// convert client connection to a tls server
	if mitmClient {
		host := clientChannel.Header.Host
		srvConfig := tls.Config{
			GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
				name := info.ServerName
				if name == "" {
					name = host
				}
				return p.runtime.certs.GetCertificate(name)
			},
		}
		clientChannel.SetConn(tls.Server(clientChannel.Conn(), &srvConfig))
	}
	// convert proxy connection to a tls client
	if mitmProxy {
		clientCfg := tls.Config{InsecureSkipVerify: true}
		proxyChannel.SetConn(tls.Client(proxyChannel.Conn(), &clientCfg))
	}
	// infinite double pipe sync
	for {
		p.logLine = ""
		p.logPrefix = ""
		clientChannel.SetPrefix("")
		err := clientChannel.ReadRequestHeaders()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = clientChannel.BadRequest()
			break
		}
		p.computeLog(clientChannel, rule, firstProxy, firstHostPort)
		if p.verbose {
			p.runtime.printer.Infof("%s", p.logLine)
		}
		if p.conf.Debug {
			prefix := fmt.Sprintf("%s C<", p.logPrefix)
			for _, header := range clientChannel.Header.Headers {
				p.runtime.printer.ReqHeaderf("%s %s", prefix, header)
			}
		}
		err = p.forwardRequest(clientChannel, proxyChannel, *firstProxy.Type, nil)
		if err != nil {
			break
		}
		if p.conf.Debug {
			proxyChannel.SetPrefix(fmt.Sprintf("%s P<", p.logPrefix))
		}
		err = proxyChannel.ReadResponseHeaders(true)
		if err != nil {
			break
		}
		err = p.forwardResponse(proxyChannel, clientChannel, false)
		if err != nil {
			break
		}
	}
	return p.closeChannels(clientChannel, proxyChannel)
}
