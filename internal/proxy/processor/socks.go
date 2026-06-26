package processor

import (
	"crypto/tls"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/txthinking/socks5"
	netproxy "golang.org/x/net/proxy"

	"test/internal/config"
	"test/internal/proxy/message"
	"test/internal/proxy/transport"
)

// connectSocks establishes a connection through an upstream SOCKS5 proxy
// (axis A = socks). When authenticated, it derives the SOCKS auth from the
// authorization (filled from authFunc), and tunnels to the target (a CONNECT
// is simulated for client CONNECT requests).
func (p *Process) connectSocks(client *message.ProxyRequest, rule *config.Rule, firstProxy *config.Proxy, firstHostPort string, dialer *net.Dialer, authentication bool, authFunc func() (*string, error), authorization **string) (net.Conn, bool, error) {
	simulateConnect := client.Header.IsConnect
	var authz *netproxy.Auth
	if authentication {
		*authorization, _ = authFunc()
	}
	if *authorization != nil {
		userDetails := strings.SplitN(**authorization, ":", 2)
		authz = &netproxy.Auth{User: userDetails[0], Password: userDetails[1]}
	}
	socks, err := netproxy.SOCKS5("tcp4", firstHostPort, authz, dialer)
	if err != nil {
		return nil, simulateConnect, err
	}
	hostPort := client.Header.HostPort
	h, port := splitHostPort(hostPort, "", "", false)
	if rule.Dns != nil {
		h2, p2 := splitHostPort(*rule.Dns, h, port, false)
		hostPort = h2 + ":" + p2
	}
	conn, err := socks.Dial("tcp4", hostPort)
	return conn, simulateConnect, err
}

// ProcessSocks handles an inbound SOCKS5 request (when this proxy acts as a
// SOCKS server): it matches the target, selects the upstream, dials it
// (directly or through an upstream SOCKS proxy) and pipes both directions.
func (p *Process) ProcessSocks(request *socks5.Request) {
	var err error
	p.trace("start process")

	// find matching rule and proxy
	requestHostPort := request.Address()
	rule, proxies := p.runtime.router.MatchSocks(requestHostPort)
	firstProxy, firstHostPort := p.runtime.selector.FindFirstProxy(rule, proxies)
	proxyName := "none"
	if firstProxy != nil {
		proxyName = firstProxy.Name
	}
	if firstHostPort == "" {
		firstHostPort = requestHostPort
	}

	if firstProxy != nil {
		p.trace("proxy matched '%s'", proxyName)
	} else {
		p.trace("no proxy matched")
	}

	// verbosity
	verbose := p.runtime.conf.Verbose
	if rule != nil && firstProxy != nil {
		verbose = firstProxy.Verbose
	}
	if rule != nil {
		verbose = rule.Verbose
	}
	verbose = verbose || p.runtime.conf.Debug || p.runtime.conf.Trace
	p.verbose = verbose

	// verbose log
	if p.verbose {
		p.runtime.printer.Infof("[%s] socks %s => %s", proxyName, requestHostPort, firstHostPort)
	}

	// if no proxy, just throw away the request
	if rule == nil || firstProxy == nil || *firstProxy.Type == config.ProxyNone {
		return
	}

	// check if authentication is required as defined in the configuration
	authentication := *firstProxy.Type == config.ProxySocks && firstProxy.Cred != nil

	var authorization *string
	var authorizationFunc func() (*string, error)

	if authentication {
		p.trace("authentication")
		var authenticated bool
		authenticated, _, authorizationFunc = p.computeAuthPerConf(firstProxy)
		if !authenticated {
			return
		}
	}

	// allow 3 retries, creating a new remote connection each time
	retryable := 3
	clientChannel := message.NewProxyRequest(p.conn, p.runtime.printer, "")
	var proxyChannel *message.ProxyRequest
	for {
		p.trace("start connection (retryable=%d)", retryable)
		var conn net.Conn
		dialer := new(net.Dialer)
		dialer.Timeout = time.Duration(p.runtime.conf.ConnectTimeout) * time.Second
		switch *firstProxy.Type {
		case config.ProxySocks:
			var authz *netproxy.Auth
			if authentication {
				authorization, _ = authorizationFunc()
			}
			if authorization != nil {
				userDetails := strings.SplitN(*authorization, ":", 2)
				authz = &netproxy.Auth{User: userDetails[0], Password: userDetails[1]}
			}
			var socks netproxy.Dialer
			socks, err = netproxy.SOCKS5("tcp4", firstHostPort, authz, dialer)
			if err == nil {
				hostPort := requestHostPort
				h, port := splitHostPort(hostPort, "", "", false)
				if rule.Dns != nil {
					h2, p2 := splitHostPort(*rule.Dns, h, port, false)
					hostPort = h2 + ":" + p2
				}
				conn, err = socks.Dial("tcp4", hostPort)
			}
		case config.ProxyDirect:
			hostPort := requestHostPort
			host, port := splitHostPort(hostPort, "", "", false)
			if rule.Dns != nil {
				h2, p2 := splitHostPort(*rule.Dns, host, port, false)
				hostPort = h2 + ":" + p2
			}
			if firstProxy.Ssl {
				conn, err = tls.DialWithDialer(dialer, "tcp4", hostPort, &tls.Config{})
			} else {
				conn, err = dialer.Dial("tcp4", hostPort)
			}
		}
		if err != nil {
			p.runtime.printer.Errorf("[%s] socks %s => %s: dial %#s", proxyName, requestHostPort, firstHostPort, err)
			retryable--
			if retryable > 0 {
				continue
			}
			return
		}
		transport.ConfigureConn(conn)
		proxyChannel = message.NewProxyRequest(conn, p.runtime.printer, "")
		break
	}
	// create a wait group to wait for both to finish, double pipe async copy
	var finished sync.WaitGroup
	finished.Add(2)
	go p.pipe(clientChannel, proxyChannel, &finished)
	go p.pipe(proxyChannel, clientChannel, &finished)
	finished.Wait()
}
