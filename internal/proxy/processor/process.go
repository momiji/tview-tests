package processor

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/palantir/stacktrace"

	"test/internal/config"
	"test/internal/proxy/message"
	"test/internal/proxy/router"
	"test/internal/proxy/transport"
	"test/internal/proxy/upstream"
	"test/internal/service/printer"
)

// Process handles a single client connection. It is single-threaded (one
// goroutine), except for the duplex pipe which uses two.
type Process struct {
	runtime *Runtime
	// snapshot captured at creation, so a mid-connection reload does not change
	// the config/router/selector under a running Process
	conf        *config.ProxyConf
	router      *router.Router
	selector    *upstream.Selector
	conn        net.Conn // client connection (a transport.TrafficConn)
	trafficConn *transport.TrafficConn
	reqId       int32
	verbose     bool
	logName     string
	logPrefix   string
	logLine     string
	logTraffic  string
	logHostPort string
	loadCounter int32
	ti          *printer.ReqLogInfo
	meter       transport.TrafficMeter
}

func NewProcess(runtime *Runtime, conn net.Conn) *Process {
	reqId := runtime.newReqId()
	ti := printer.NewReqLogInfo(reqId, "process")
	trafficConn := transport.NewTrafficConn(conn)
	snap := runtime.current.Load()
	return &Process{
		runtime:     runtime,
		conf:        snap.conf,
		router:      snap.router,
		selector:    snap.selector,
		conn:        trafficConn,
		trafficConn: trafficConn,
		reqId:       reqId,
		loadCounter: runtime.LoadCounter(),
		ti:          ti,
	}
}

func (p *Process) trace(format string, args ...interface{}) {
	if p.conf.Trace {
		p.runtime.printer.ReqInfof(p.ti, format, args...)
	}
}

func (p *Process) ProcessHttp() {
	// automatically close connection on exit
	defer func() { _ = p.conn.Close() }()
	clientChannel := message.NewProxyRequest(p.conn, p.runtime.printer, "")
	// loop until proxyChannel is empty, meaning connection should close
	var proxyChannel *message.ProxyRequest
	for !p.runtime.stopped() {
		// we don't reuse the proxyChannel as the target can change
		proxyChannel = p.processChannel(clientChannel, nil)
		if proxyChannel == nil {
			break
		}
	}
}

func (p *Process) processChannel(clientChannel, proxyChannel *message.ProxyRequest) *message.ProxyRequest {
	p.trace("start process")
	p.logLine = ""
	p.logPrefix = ""
	clientChannel.SetPrefix("")

	// set timeout for reading headers - prevent waiting forever for incoming http headers
	_ = p.conn.SetReadDeadline(time.Now().Add(time.Duration(p.conf.ConnectTimeout) * time.Second))

	err := clientChannel.ReadRequestHeaders()
	if err != nil {
		if err == io.EOF {
			return p.closeChannels(clientChannel, proxyChannel)
		}
		_ = clientChannel.BadRequest()
		return p.closeChannels(clientChannel, proxyChannel)
	}

	// is url for local web server?
	if len(clientChannel.Header.Url) > 0 && clientChannel.Header.Url[0] == '/' {
		_ = p.webServer(clientChannel)
		return p.closeChannels(clientChannel, proxyChannel)
	}

	// prevent timeout on connections
	_ = p.conn.SetReadDeadline(time.Time{})

	// find proxy to use from host:port
	p.trace("proxy match")
	rule, proxies := p.router.MatchHttp(clientChannel.Header.Url, clientChannel.Header.HostPort)
	firstProxy, firstHostPort := p.selector.FindFirstProxy(rule, proxies)
	if firstProxy != nil {
		p.trace("proxy matched '%s'", firstProxy.Name)
	} else {
		p.trace("no proxy matched")
	}

	// print log in verbose mode
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

	// traffic data
	p.meter = p.runtime.traffic.New(p.reqId, p.logTraffic)
	if p.meter != nil {
		p.trafficConn.SetMeter(p.meter)
	}

	// if no proxy, just throw away the request
	if rule == nil || firstProxy == nil || *firstProxy.Type == config.ProxyNone {
		_ = clientChannel.BadRequest()
		return p.closeChannels(clientChannel, proxyChannel)
	}

	// authorization is computed lazily, as we don't know yet if we need a new authorization header.
	var authorization *string
	var authorizationFunc func() (*string, error)
	var authorizationContext string

	// check if authentication is required as defined in the configuration.
	authentication := (*firstProxy.Type == config.ProxyKerberos || *firstProxy.Type == config.ProxyBasic || *firstProxy.Type == config.ProxySocks) && firstProxy.Cred != nil

	// if proxy requires per-user authentication
	if authentication && firstProxy.Cred.IsPerUser {
		p.trace("per-user authentication")
		proxyAuthorization := clientChannel.FindHeader("proxy-authorization")
		if proxyAuthorization != nil {
			var authenticated bool
			authenticated, authorizationContext, authorizationFunc = p.computeAuthPerUser(firstProxy, proxyAuthorization)
			if !authenticated {
				_ = clientChannel.RequireAuth(firstProxy.Name)
				return p.closeChannels(clientChannel, proxyChannel)
			}
		} else {
			_ = clientChannel.RequireAuth(firstProxy.Name)
			return p.closeChannels(clientChannel, proxyChannel)
		}
	}

	// if proxy is not per-user
	if authentication && !firstProxy.Cred.IsPerUser {
		p.trace("authentication")
		var authenticated bool
		authenticated, authorizationContext, authorizationFunc = p.computeAuthPerConf(firstProxy)
		if !authenticated {
			_ = clientChannel.RequireAuth(firstProxy.Name)
			return p.closeChannels(clientChannel, proxyChannel)
		}
	}
	_ = authorizationContext

	// simulateConnect: for peers that do not talk HTTP like DIRECT or SOCKS
	simulateConnect := false
	// allow 3 retries, creating a new remote connection each time
	retryable := 3
	// man-in-the-middle
	mitmProxy := true
	mitmClient := true
	// try up to retryable connections
	for {
		p.trace("start connection (retryable=%d)", retryable)
		if proxyChannel == nil {
			p.trace("create proxy channel")
			var conn net.Conn
			conn, simulateConnect, err = p.connectUpstream(clientChannel, firstProxy, firstHostPort, rule, authentication, authorizationFunc, &authorization)
			if err != nil {
				p.runtime.printer.Errorf("%s => dial: %#s", p.logLine, err)
				return p.closeChannels(clientChannel, proxyChannel)
			}
			// if conn is nil - proxyType=PAC and PAC not downloaded, so it did not resolve to an other proxy
			if conn == nil {
				p.runtime.printer.Infof("%s => dial: no connection available", p.logLine)
				return p.closeChannels(clientChannel, proxyChannel)
			}
			transport.ConfigureConn(conn)
			proxyChannel = message.NewProxyRequest(conn, p.runtime.printer, "")
		}

		// get authorization header
		if authentication && authorization == nil {
			authorization, err = authorizationFunc()
			if err != nil {
				p.runtime.printer.Infof("[-] Shutting down to prevent locking user account with repeated invalid password...")
				p.runtime.Stop()
				return nil
			}
			if authorization == nil {
				authorization = &noAuth
			}
		}

		// forward request to proxy
		if !simulateConnect {
			p.trace("forward request")
			if !clientChannel.Header.DirectToConnect {
				err = p.forwardRequest(clientChannel, proxyChannel, *firstProxy.Type, authorization)
				if err != nil {
					p.runtime.printer.Errorf("%s => forward: %#s", p.logLine, err)
					return p.closeChannels(clientChannel, proxyChannel)
				}
			} else {
				err = p.forwardConnect(clientChannel, proxyChannel, *firstProxy.Type, authorization)
				if err != nil {
					p.runtime.printer.Errorf("%s => forward: %#s", p.logLine, err)
					return p.closeChannels(clientChannel, proxyChannel)
				}
				if p.conf.Debug {
					proxyChannel.SetPrefix(fmt.Sprintf("%s P<", p.logPrefix))
				}
				err = proxyChannel.ReadResponseHeaders(false)
				if err != nil {
					p.runtime.printer.Errorf("%s => forward: %#s", p.logLine, err)
					return p.closeChannels(clientChannel, proxyChannel)
				}
				if strings.ToLower(proxyChannel.Header.Reason) != "connection established" {
					err = errors.New("connection not established")
					p.runtime.printer.Errorf("%s => forward: %#s", p.logLine, err)
					return p.closeChannels(clientChannel, proxyChannel)
				}
				if p.conf.Debug {
					proxyChannel.SetPrefix(fmt.Sprintf("%s P>", p.logPrefix))
				}
				proxyChannel.SetConn(tls.Client(proxyChannel.Conn(), &tls.Config{ServerName: clientChannel.Header.Host}))
				err = p.forwardRequest(clientChannel, proxyChannel, *firstProxy.Type, authorization)
				if err != nil {
					p.runtime.printer.Errorf("%s => forward: %#s", p.logLine, err)
					return p.closeChannels(clientChannel, proxyChannel)
				}
			}
		}

		// read response headers
		p.trace("read response")
		if p.conf.Debug {
			proxyChannel.SetPrefix(fmt.Sprintf("%s P<", p.logPrefix))
		}
		if simulateConnect {
			// inject headers manually as if proxyChannel has been called
			_ = proxyChannel.InjectResponseHeaders([]string{"HTTP/1.0 200 Connection established"})
		} else {
			err := proxyChannel.ReadResponseHeaders(!clientChannel.Header.IsConnect)
			if err != nil {
				retryable--
				if err == io.EOF && retryable > 0 {
					p.runtime.printer.Errorf("%s => %#s", p.logLine, stacktrace.NewError("Remote connection closed, retrying"))
					p.closeChannel(proxyChannel)
					proxyChannel = nil
					continue
				} else if err == io.EOF {
					p.runtime.printer.Errorf("%s => %#s", p.logLine, stacktrace.NewError("Remote connection closed"))
				} else {
					p.runtime.printer.Errorf("%s => response: %#s", p.logLine, err)
				}
				_ = clientChannel.BadRequest()
				return p.closeChannels(clientChannel, proxyChannel)
			}
		}
		break
	}

	// downgrade version if proxy is lower than client
	if proxyChannel.Header.Version.Order() < clientChannel.Header.Version.Order() {
		clientChannel.Header.Version = proxyChannel.Header.Version
	}
	clientChannel.Header.KeepAlive = clientChannel.Header.KeepAlive && proxyChannel.Header.KeepAlive

	// forward response to client
	p.trace("forward response")
	err = p.forwardResponse(proxyChannel, clientChannel, authentication)
	if err != nil {
		return p.closeChannels(clientChannel, proxyChannel)
	}

	// man-in-the-middle: decode https flows
	if clientChannel.Header.IsConnect && rule.Mitm && (mitmProxy || mitmClient) {
		return p.transportMitm(clientChannel, proxyChannel, rule, firstProxy, firstHostPort, mitmClient, mitmProxy)
	}
	// treat CONNECT as a forever duplex pipe
	if clientChannel.Header.IsConnect || proxyChannel.Header.Status == 100 {
		p.trace("duplex pipe forever")
		return p.transportTunnel(clientChannel, proxyChannel)
	}
	// if KeepAlive, allow to reuse the client connection for the next request
	if clientChannel.Header.KeepAlive && proxyChannel.Header.ContentLength != -2 {
		// only if config has not changed
		if p.loadCounter == p.runtime.LoadCounter() {
			return proxyChannel
		}
		return p.closeChannels(clientChannel, proxyChannel)
	}
	// else, return
	return p.closeChannels(clientChannel, proxyChannel)
}

func (p *Process) computeLog(channel *message.ProxyRequest, rule *config.Rule, proxy *config.Proxy, hostPort string) {
	if p.logLine != "" {
		return
	}
	// compute verbosity: rule overrides proxy overrides conf, then debug/trace force on
	verbose := p.conf.Verbose
	if rule != nil && proxy != nil {
		verbose = proxy.Verbose
	}
	if rule != nil {
		verbose = rule.Verbose
	}
	verbose = verbose || p.conf.Debug || p.conf.Trace
	p.verbose = verbose
	// compute proxy display name
	name := config.ProxyNone.Name()
	if rule != nil {
		name = proxyShortName(*rule.Proxy)
		if name != proxy.Name {
			name = name + ">" + proxy.Name
		}
	}
	// compute log line
	p.logName = name
	p.logPrefix = fmt.Sprintf("(%d) [%s]", p.reqId, name)
	p.logLine = fmt.Sprintf("%s %s %s HTTP/%s", p.logPrefix, channel.Header.Method, channel.Header.OriginalUrl, channel.Header.Version)
	p.logTraffic = fmt.Sprintf("%s %s %s HTTP/%s", name, channel.Header.Method, channel.Header.OriginalUrl, channel.Header.Version)
	if channel.Header.HostEmpty {
		p.logLine = fmt.Sprintf("%s (%s)", p.logLine, channel.Header.Host)
	}
	p.logHostPort = hostPort
}

func (p *Process) webServer(channel *message.ProxyRequest) error {
	line := channel.Header.Method + " " + channel.Header.Url
	if !strings.HasPrefix(strings.ToLower(line), "get /proxy.pac") {
		return channel.NotFound()
	}
	err := channel.WriteStatusLine(message.Http10, 200, "OK")
	if err != nil {
		return err // no wrap
	}
	err = channel.WriteDateHeader()
	if err != nil {
		return err // no wrap
	}
	return channel.WriteContent(p.router.Pac(), false, message.CT_PLAIN_UTF8)
}

func (p *Process) closeChannels(clientChannel, proxyChannel *message.ProxyRequest) *message.ProxyRequest {
	p.trace("close channels")
	p.closeChannel(clientChannel)
	p.closeChannel(proxyChannel)
	if p.meter != nil {
		p.runtime.traffic.Remove(p.meter)
	}
	return nil
}

func (p *Process) closeChannel(channel *message.ProxyRequest) {
	if channel == nil {
		return
	}
	_ = channel.Conn().Close()
}
