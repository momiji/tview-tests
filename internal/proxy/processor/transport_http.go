package processor

import (
	"fmt"
	"io"
	"strings"

	"test/internal/config"
	"test/internal/proxy/message"
	"test/internal/proxy/transport"
)

// debugHeaderPrefix is the prefix of the diagnostic response headers added
// (and stripped from upstream) in debug mode. Neutral, not tied to a product
// name (see IDEAS.md).
const debugHeaderPrefix = "x-proxy"

// forwardRequest writes the client request to the upstream (axis C = http):
// request line, filtered headers, optional Proxy-Authorization, then the body.
func (p *Process) forwardRequest(clientChannel *message.ProxyRequest, proxyChannel *message.ProxyRequest, proxyType config.ProxyType, auth *string) error {
	var err error
	if p.conf.Debug {
		proxyChannel.SetPrefix(fmt.Sprintf("%s P>", p.logPrefix))
	}
	// request line must use absoluteUri when the target is a proxy
	if proxyType == config.ProxyDirect || proxyType == config.ProxySocks {
		err = proxyChannel.WriteRequestLine(clientChannel.Header.Method, clientChannel.Header.RelativeUrl, clientChannel.Header.Version)
	} else {
		err = proxyChannel.WriteRequestLine(clientChannel.Header.Method, clientChannel.Header.LineUrl, clientChannel.Header.Version)
	}
	if err != nil {
		return err // no wrap
	}
	expectContinue := false
	for _, header := range clientChannel.Header.Headers[1:] {
		lower := strings.ToLower(header)
		switch {
		case strings.HasPrefix(lower, "proxy-connection:") || strings.HasPrefix(lower, "connection:"):
			continue
		case strings.HasPrefix(lower, "proxy-authorization:"):
			continue
		case strings.HasPrefix(lower, "expect") && strings.Contains(lower, "100-continue") && strings.ToUpper(clientChannel.Header.Method) == "PUT":
			expectContinue = true
		}
		err = proxyChannel.WriteHeaderLine(header)
		if err != nil {
			return err // no wrap
		}
	}
	if auth != nil {
		err = proxyChannel.WriteHeader("Proxy-Authorization", *auth)
		if err != nil {
			return err // no wrap
		}
	}
	err = proxyChannel.WriteKeepAlive(clientChannel.Header.KeepAlive, proxyType != config.ProxyDirect && proxyType != config.ProxySocks)
	if err != nil {
		return err // no wrap
	}
	err = proxyChannel.CloseHeader()
	if err != nil {
		return err // no wrap
	}
	// special PUT with Expect: 100-continue
	if expectContinue {
		return nil
	}
	return p.forwardStream(clientChannel, proxyChannel)
}

// forwardConnect writes a CONNECT request to the upstream proxy (used for the
// direct-to-CONNECT upgrade, /~/ path).
func (p *Process) forwardConnect(clientChannel *message.ProxyRequest, proxyChannel *message.ProxyRequest, _ config.ProxyType, auth *string) error {
	var err error
	if p.conf.Debug {
		proxyChannel.SetPrefix(fmt.Sprintf("%s P>", p.logPrefix))
	}
	err = proxyChannel.WriteRequestLine("CONNECT", clientChannel.Header.HostPort, clientChannel.Header.Version)
	if err != nil {
		return err // no wrap
	}
	err = proxyChannel.WriteHeader("Host", clientChannel.Header.HostPort)
	if err != nil {
		return err // no wrap
	}
	for _, header := range clientChannel.Header.Headers[1:] {
		lower := strings.ToLower(header)
		if strings.HasPrefix(lower, "user-agent:") {
			err = proxyChannel.WriteHeaderLine(header)
			if err != nil {
				return err // no wrap
			}
		}
	}
	err = proxyChannel.WriteKeepAlive(clientChannel.Header.KeepAlive, true)
	if err != nil {
		return err // no wrap
	}
	if auth != nil {
		err = proxyChannel.WriteHeader("Proxy-Authorization", *auth)
		if err != nil {
			return err // no wrap
		}
	}
	err = proxyChannel.CloseHeader()
	if err != nil {
		return err // no wrap
	}
	return p.forwardStream(clientChannel, proxyChannel)
}

// forwardResponse writes the upstream response back to the client (axis C =
// http), filtering hop-by-hop headers and the diagnostic headers.
func (p *Process) forwardResponse(proxyChannel *message.ProxyRequest, clientChannel *message.ProxyRequest, authentication bool) error {
	var err error
	if p.conf.Debug {
		clientChannel.SetPrefix(fmt.Sprintf("%s C>", p.logPrefix))
	}
	for _, header := range proxyChannel.Header.Headers {
		lower := strings.ToLower(header)
		switch {
		case strings.HasPrefix(lower, "connection:"):
			continue
		case strings.HasPrefix(lower, "proxy-connection:"):
			continue
		case strings.HasPrefix(lower, "proxy-authenticate:") && authentication:
			continue
		case strings.HasPrefix(lower, debugHeaderPrefix+"-") && p.conf.Debug:
			continue
		}
		err = clientChannel.WriteHeaderLine(header)
		if err != nil {
			return err // no wrap
		}
	}
	if p.conf.Debug {
		_ = clientChannel.WriteHeader(debugHeaderPrefix+"-name", p.logName)
		_ = clientChannel.WriteHeader(debugHeaderPrefix+"-host", p.logHostPort)
	}
	if !clientChannel.Header.IsConnect {
		err = clientChannel.WriteKeepAlive(clientChannel.Header.KeepAlive, clientChannel.Header.IsProxyConnection)
		if err != nil {
			return err // no wrap
		}
	}
	err = clientChannel.CloseHeader()
	if err != nil {
		return err // no wrap
	}
	// special response with no body
	if strings.ToUpper(clientChannel.Header.Method) == "HEAD" ||
		(proxyChannel.Header.Status >= 100 && proxyChannel.Header.Status < 200) ||
		proxyChannel.Header.Status == 204 ||
		proxyChannel.Header.Status == 304 {
		return nil
	}
	return p.forwardStream(proxyChannel, clientChannel)
}

// forwardStream copies the message body from source to target, handling the
// content-length / chunked / EOF-delimited cases.
func (p *Process) forwardStream(source *message.ProxyRequest, target *message.ProxyRequest) error {
	dataReader := strings.NewReader(string(source.Header.Data))
	sourceReader := io.MultiReader(dataReader, source.Conn())
	var reader io.Reader
	if source.Header.ContentLength == -2 {
		// No content-length, read until EOF
		reader = sourceReader
	} else if source.Header.ContentLength == -1 {
		// chunked: keep the chunk lines (instrumented reader)
		reader = transport.NewChunkedReader(sourceReader)
	} else {
		reader = io.LimitReader(sourceReader, source.Header.ContentLength)
	}
	_, err := io.Copy(target.Conn(), reader)
	return err // no wrap
}
