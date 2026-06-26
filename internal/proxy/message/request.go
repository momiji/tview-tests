package message

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/palantir/stacktrace"

	"test/internal/service/printer"
)

const CT_PLAIN_UTF8 = "text/plain; charset=UTF-8"

// ProxyRequest reads and writes the HTTP messages on a single connection. It
// logs header lines through the injected Printer when prefix is non-empty.
type ProxyRequest struct {
	// input / output stream
	conn net.Conn
	// parsed headers, with any already-read body bytes
	Header *RequestHeader
	// verbose header-logging prefix; empty disables logging
	prefix  string
	printer *printer.Printer
}

func NewProxyRequest(conn net.Conn, p *printer.Printer, prefix string) *ProxyRequest {
	return &ProxyRequest{conn: conn, printer: p, prefix: prefix}
}

// SetPrefix changes the header-logging prefix (empty disables logging).
func (r *ProxyRequest) SetPrefix(prefix string) {
	r.prefix = prefix
}

// Conn returns the underlying connection (used for raw piping).
func (r *ProxyRequest) Conn() net.Conn {
	return r.conn
}

// SetConn replaces the underlying connection, e.g. to wrap it in TLS for a
// CONNECT upgrade or MITM.
func (r *ProxyRequest) SetConn(conn net.Conn) {
	r.conn = conn
}

func (r *ProxyRequest) injectHeaders(headers []string) (*RequestHeader, error) {
	if r.prefix != "" {
		for _, header := range headers {
			r.printer.ReqHeaderf("%s %s", r.prefix, header)
		}
	}
	h := make([]string, len(headers))
	copy(h, headers)
	d := make([]byte, 0)
	rh := RequestHeader{Headers: h, Data: d}
	r.Header = &rh
	return r.Header, nil
}

func (r *ProxyRequest) readFull(buffer []byte) (int, error) {
	var err error
	length := 0
	read := 0
	pos := 0
	found := 0
	b := buffer
	for {
		read, err = r.conn.Read(b)
		length += read
		if err != nil {
			return length, err // no wrap
		}
		// looking for \r\n\r\n (4 chars) at the end of http headers
		for i := pos; i < length; i++ {
			if buffer[i] == 13 || buffer[i] == 10 {
				found++
				if found == 4 {
					return length, nil
				}
			} else {
				found = 0
			}
		}
		pos = length
		b = buffer[length:]
	}
}

func (r *ProxyRequest) readHeaders() (*RequestHeader, error) {
	// init header
	h := RequestHeader{}
	// read header bytes
	startData := 0
	startLine := 0
	buffer := make([]byte, HeaderMaxSize)
	headers := make([]string, 0, 32)
	// read headers
	readLen, err := r.readFull(buffer)
	if err != nil {
		if readLen == 0 || err == io.EOF {
			return nil, io.EOF
		}
		return nil, stacktrace.Propagate(err, "Could not read headers")
	}
	if readLen == 0 {
		return nil, stacktrace.NewError("Invalid request, no headers")
	}
	for i := 0; i < readLen; i++ {
		if buffer[i] == '\r' && i+1 < readLen && buffer[i+1] == '\n' {
			// if this is an empty line, headers are finished
			if i == startLine {
				startData = i + 2
				break
			}
			// otherwise this is a new header line
			header := buffer[startLine:i]
			if r.prefix != "" {
				r.printer.ReqHeaderf("%s %s", r.prefix, string(header))
			}
			startLine = i + 2
			headers = append(headers, string(header))
			i++
		}
	}
	// save headers
	if len(headers) == 0 {
		return nil, stacktrace.NewError("Invalid request, no headers")
	}
	h.Headers = headers
	h.Data = buffer[startData:readLen]
	h.StartData = startData
	r.Header = &h
	return &h, nil
}

func (r *ProxyRequest) ReadRequestHeaders() error {
	rh, err := r.readHeaders()
	if err != nil {
		return err // no wrap
	}
	err = rh.analyseRequestLine()
	if err != nil {
		return err // no wrap
	}
	err = rh.analyseHeaders(true, false)
	if err != nil {
		return err // no wrap
	}
	return nil
}

func (r *ProxyRequest) ReadResponseHeaders(allowEOFDelimitedBody bool) error {
	rh, err := r.readHeaders()
	if err != nil {
		return err // no wrap
	}
	err = rh.analyseResponseLine()
	if err != nil {
		return err // no wrap
	}
	err = rh.analyseHeaders(false, allowEOFDelimitedBody)
	if err != nil {
		return err // no wrap
	}
	return nil
}

func (r *ProxyRequest) InjectResponseHeaders(headers []string) error {
	rh, err := r.injectHeaders(headers)
	if err != nil {
		return err // no wrap
	}
	err = rh.analyseResponseLine()
	if err != nil {
		return err // no wrap
	}
	err = rh.analyseHeaders(false, false)
	if err != nil {
		return err // no wrap
	}
	return nil
}

func (r *ProxyRequest) FindHeader(s string) *string {
	for _, header := range r.Header.Headers {
		kv := strings.SplitN(header, ":", 2)
		if strings.ToLower(kv[0]) == strings.ToLower(s) {
			val := strings.TrimSpace(kv[1])
			return &val
		}
	}
	return nil
}

func (r ProxyRequest) WriteStatusLine(version HttpVersion, status int, reason string) error {
	return r.WriteHeaderLine(fmt.Sprintf("HTTP/%s %d %s", version.Version(), status, reason))
}

func (r ProxyRequest) WriteDateHeader() error {
	return r.WriteHeader("Date", time.Now().Format(time.RFC1123))
}

func (r ProxyRequest) WriteHeader(key, val string) error {
	return r.WriteHeaderLine(fmt.Sprintf("%s: %s", key, val))
}

func (r ProxyRequest) WriteKeepAlive(keepAlive bool, isProxy bool) error {
	header := "Connection"
	if isProxy {
		header = "Proxy-Connection"
	}
	if keepAlive {
		return r.WriteHeader(header, "keep-alive")
	} else {
		return r.WriteHeader(header, "close")
	}
}

func (r ProxyRequest) CloseHeader() error {
	return r.WriteHeaderLine("")
}

func (r ProxyRequest) WriteContent(content string, keepAlive bool, contentType string) error {
	err := r.WriteHeader("Content-Length", strconv.Itoa(len(content)))
	if err != nil {
		return err // no wrap
	}
	err = r.WriteHeader("Content-Type", contentType)
	if err != nil {
		return err // no wrap
	}
	err = r.WriteKeepAlive(keepAlive, false)
	if err != nil {
		return err // no wrap
	}
	err = r.CloseHeader()
	_, err = r.conn.Write([]byte(content))
	return err // no wrap
}

func (r ProxyRequest) BadRequest() error {
	err := r.WriteStatusLine(Http10, 400, "Bad Request")
	if err != nil {
		return err // no wrap
	}
	err = r.WriteDateHeader()
	if err != nil {
		return err // no wrap
	}
	return r.WriteContent("Bad Request\n", false, CT_PLAIN_UTF8)
}

func (r ProxyRequest) NotFound() error {
	err := r.WriteStatusLine(Http10, 404, "Not Found")
	if err != nil {
		return err // no wrap
	}
	err = r.WriteDateHeader()
	if err != nil {
		return err // no wrap
	}
	return r.WriteContent("Not Found\n", false, CT_PLAIN_UTF8)
}

func (r *ProxyRequest) RequireAuth(proxy string) error {
	err := r.WriteStatusLine(Http10, 407, "Proxy Authentication Required")
	if err != nil {
		return err // no wrap
	}
	err = r.WriteDateHeader()
	if err != nil {
		return err // no wrap
	}
	err = r.WriteHeader("Proxy-Authenticate", fmt.Sprintf("Basic realm=\"Authentication required for '%s', use DOMAIN\\USERNAME or USERNAME@DOMAIN or USERNAME\"", proxy))
	if err != nil {
		return err // no wrap
	}
	return r.WriteContent("Proxy Authentication Required\n", false, CT_PLAIN_UTF8)
}

func (r *ProxyRequest) WriteRequestLine(method string, url string, version HttpVersion) error {
	return r.WriteHeaderLine(fmt.Sprintf("%s %s HTTP/%s", method, url, version.Version()))
}

func (r ProxyRequest) WriteHeaderLine(line string) error {
	if r.prefix != "" && line != "" {
		r.printer.ReqHeaderf("%s %s", r.prefix, line)
	}
	_, err := r.conn.Write([]byte(fmt.Sprintf("%s\r\n", line)))
	return err // no wrap
}
