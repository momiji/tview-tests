// Package message reads, parses and writes the HTTP messages exchanged by
// the proxy. ProxyRequest wraps a connection and turns raw header bytes into
// a parsed RequestHeader (request or response line + headers), and writes
// status lines, headers and canned responses back.
package message

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/palantir/stacktrace"
)

// HeaderMaxSize bounds how many bytes are buffered while reading request
// headers.
const HeaderMaxSize = 32 * 1024

// RequestHeader holds a parsed request or response: the raw header lines, any
// body bytes already read past the headers, and the fields extracted from the
// first line and the headers.
type RequestHeader struct {
	Headers   []string
	Data      []byte
	StartData int
	// request line
	Method          string
	RelativeUrl     string      // relative url without proto://host(:port), starts with /
	Url             string      // request url, with additional proto://host(:port) - may be altered for altered url
	OriginalUrl     string      // request url, with additional proto://host(:port) - not altered
	LineUrl         string      // url as passed in first header line - may be altered for altered url
	Version         HttpVersion // http version
	IsConnect       bool        // request line is CONNECT
	IsSsl           bool        // request url is https, but not not implies CONNECT is used
	Host            string      // host, without port number
	Port            int         // port number
	HostPort        string      // host with port number
	HostEmpty       bool        // host from line is empty
	DirectToConnect bool        // direct use of proxy requires upgrade to CONNECT
	// response line
	Status int
	Reason string
	// headers
	KeepAlive         bool
	ContentLength     int64
	IsProxyConnection bool
}

type HttpVersion string

const (
	Http10 HttpVersion = "1.0"
	Http11 HttpVersion = "1.1"
	Http2  HttpVersion = "2"
)

var HttpVersions = [...]HttpVersion{Http10, Http11, Http2}

func GetHttpVersion(version string) HttpVersion {
	a := strings.Split(version, "/")
	if len(a) == 0 {
		return Http10
	}
	v := a[len(a)-1]
	for _, hv := range HttpVersions {
		if v == hv.Version() {
			return hv
		}
	}
	return Http10
}

func (hv HttpVersion) Version() string {
	return string(hv)
}

func (hv HttpVersion) Order() int {
	for i, v := range HttpVersions {
		if v == hv {
			return i
		}
	}
	return -1
}

func (rh *RequestHeader) analyseRequestLine() error {
	var err error
	// analyse first line
	headerLine := rh.Headers[0]
	line := strings.Split(headerLine, " ")
	if len(line) != 3 {
		return stacktrace.NewError("Invalid request line, expecting 'METHOD URL VERSION': %v", headerLine)
	}
	rh.Method = line[0]
	rh.Url = line[1]
	rh.LineUrl = line[1]
	rh.Version = GetHttpVersion(line[2])
	if strings.ToUpper(rh.Method) == "CONNECT" {
		rh.IsConnect = true
		hp := strings.Split(rh.Url, ":")
		if len(hp) == 0 || len(hp) > 2 {
			return stacktrace.NewError("Invalid request line, expecting 'CONNECT host[:port] VERSION': %v", headerLine)
		}
		rh.Host = hp[0]
		rh.Port = 443
		rh.IsSsl = false
		if len(hp) == 2 {
			rh.Port, err = strconv.Atoi(hp[1])
			if err != nil {
				return stacktrace.Propagate(err, "Invalid request line, expecting 'CONNECT host[:port] VERSION': %v", headerLine)
			}
		}
		rh.Url = "https://" + rh.Host
		if rh.Port != 443 {
			rh.Url += ":" + strconv.Itoa(rh.Port)
		}
	} else {
		rh.IsConnect = false
		u, err := url.Parse(rh.Url)
		if err != nil {
			return stacktrace.Propagate(err, "Invalid request line, expecting 'METHOD URL VERSION': %v", headerLine)
		}
		rh.RelativeUrl = u.RequestURI()
		hp := strings.Split(u.Host, ":")
		if len(hp) == 0 || len(hp) > 2 {
			return stacktrace.NewError("Invalid request line, expecting 'METHOD URL VERSION': %v", headerLine)
		}
		rh.Host = hp[0]
		rh.IsSsl = strings.ToUpper(u.Scheme) == "HTTPS"
		rh.Port = 80
		if rh.IsSsl {
			rh.Port = 443
		}
		if len(hp) == 2 {
			rh.Port, err = strconv.Atoi(hp[1])
			if err != nil {
				return stacktrace.Propagate(err, "Invalid request line, expecting 'METHOD URL VERSION': %v", headerLine)
			}
		}
	}
	rh.HostPort = rh.Host + ":" + strconv.Itoa(rh.Port)
	rh.OriginalUrl = rh.Url
	rh.HostEmpty = rh.Host == ""
	return nil
}

func (rh *RequestHeader) analyseResponseLine() error {
	var err error
	// analyse first line
	headerLine := rh.Headers[0]
	line := strings.SplitN(headerLine, " ", 3)
	if len(line) != 3 {
		return stacktrace.NewError("Invalid request line, expecting 'METHOD URL VERSION': %v", headerLine)
	}
	rh.Version = GetHttpVersion(line[0])
	rh.Status, err = strconv.Atoi(line[1])
	if err != nil {
		return stacktrace.Propagate(err, "Invalid response line, expecting 'VERSION STATUS REASON': %s", line)
	}
	rh.Reason = line[2]
	return nil
}

func (rh *RequestHeader) analyseHeaders(req bool, allowEOFDelimitedBody bool) error {
	var err error
	// keep alive is the "request" default for HTTP1.1 and HTTP2
	// although RFC states that HTTP 1.1 is keep-alive by default, it is not working with windows update when going through proxy chains
	// we decided to state that connection must be closed unless the server explicitly asks for keep-alive
	rh.KeepAlive = req && (rh.Version == Http11 || rh.Version == Http2)
	hasContentLength := false
	if allowEOFDelimitedBody && !rh.IsConnect {
		// on response, content-length is -2 (until body EOF) by default, unless chunked (-1) or int value (>=0)
		rh.ContentLength = -2
	}
	// loop on headers
	for i, header := range rh.Headers {
		lower := strings.ToLower(header)
		switch {
		case rh.Host == "" && strings.HasPrefix(lower, "host:"):
			// https://www.w3.org/Protocols/rfc2616/rfc2616-sec5.html 5.1.2 => request line must use absoluteUri <=> target is a proxy
			// as rh.Host = "", call to proxy is a direct call
			// 1. url is /proxy.pac, skip
			//if rh.Url == "/proxy.pac" {
			//	continue
			//}
			// 2. url starts with /~/https://
			if strings.HasPrefix(rh.Url, "/~/") {
				paths := strings.SplitN(rh.Url, "/", 5)
				if len(paths) == 5 && (paths[2] == "http" || paths[2] == "https") {
					host, sport := splitHostPort(paths[3], "", paths[2], false)
					if sport == "http" {
						sport = "80"
					} else if sport == "https" {
						sport = "443"
					}
					rh.Host = host
					port, err := strconv.Atoi(sport)
					if err != nil {
						return stacktrace.NewError("Invalid host header: %v", header)
					}
					rh.Port = port
					rh.IsSsl = paths[2] == "https"
					rh.DirectToConnect = rh.IsSsl
					rh.RelativeUrl = "/" + paths[4]
					rh.Url = rh.RelativeUrl
					rh.HostEmpty = false
				}
			} else
			// 3. host contains either http/ or https/
			if strings.Contains(lower, "/") {
				hp := strings.SplitN(header, ":", 3)
				if len(hp) < 2 || len(hp) > 3 {
					return stacktrace.NewError("Invalid host header: %v", header)
				}
				rh.Host = strings.TrimLeft(hp[1], " ")
				// Uncomment when enabling HTTPS will be studied...
				if strings.HasPrefix(rh.Host, "http/") {
					rh.Host = rh.Host[5:]
					rh.Port = 80
					rh.IsSsl = false
				} else if strings.HasPrefix(rh.Host, "https/") {
					rh.Host = rh.Host[6:]
					rh.Port = 443
					rh.IsSsl = true
					rh.DirectToConnect = rh.IsSsl
				}
				if len(hp) > 2 {
					port, err := strconv.Atoi(hp[2])
					if err != nil {
						return stacktrace.NewError("Invalid host header: %v", header)
					}
					rh.Port = port
				}
			} else
			// 4. local web server - skip
			{
				hp := strings.SplitN(header, ":", 2)
				rh.Host = strings.TrimLeft(hp[1], " ")
				continue
			}
			//
			sport := strconv.Itoa(rh.Port)
			if rh.IsSsl {
				rh.Url = "https://" + rh.Host + rh.Url
				rh.Headers[i] = "Host: " + rh.Host
				if rh.Port != 443 {
					rh.Url = "https://" + rh.Host + ":" + sport + rh.Url
					rh.Headers[i] = "Host: " + rh.Host + ":" + sport
				}
			} else {
				rh.Url = "http://" + rh.Host + rh.Url
				rh.Headers[i] = "Host: " + rh.Host
				if rh.Port != 80 {
					rh.Url = "http://" + rh.Host + ":" + sport + rh.Url
					rh.Headers[i] = "Host: " + rh.Host + ":" + sport
				}
			}
			rh.HostPort = rh.Host + ":" + sport
			rh.LineUrl = rh.Url
		case strings.HasPrefix(lower, "content-length:") && !hasContentLength:
			hasContentLength = true
			rh.ContentLength, err = strconv.ParseInt(strings.TrimSpace(lower[15:]), 10, 64)
			if err != nil {
				return stacktrace.Propagate(err, "Invalid content-length header: %s", header)
			}
			if rh.ContentLength < 0 {
				return stacktrace.NewError("Invalid content-length header: value is < 0")
			}
		case strings.HasPrefix(lower, "transfer-encoding:"):
			if strings.Contains(lower, "chunk") {
				rh.ContentLength = -1
			}
		case strings.HasPrefix(lower, "proxy-connection:"):
			rh.IsProxyConnection = true
			fallthrough
		case strings.HasPrefix(lower, "connection:"):
			if strings.Contains(lower, "close") {
				rh.KeepAlive = false
			} else if strings.Contains(lower, "keep-alive") {
				rh.KeepAlive = true
			}
		}
	}
	return nil
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
