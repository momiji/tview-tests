package pac

import (
	"fmt"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/palantir/stacktrace"

	"test/internal/service/printer"
)

// defaultDNSTimeout bounds the DNS builtins (isResolvable, dnsResolve,
// isInNet) when PacOptions.DNSTimeout, or PacOptions itself, is unset.
const defaultDNSTimeout = time.Second

// defaultScriptTimeout bounds a single Run's script execution when
// PacOptions.ScriptTimeout, or PacOptions itself, is unset.
const defaultScriptTimeout = time.Second

// PacOptions holds configuration values for a PacExecutor, as opposed to
// collaborators like *printer.Printer which are passed to NewPac directly.
// A nil *PacOptions (or a zero-value field) falls back to the matching
// default.
type PacOptions struct {
	// DNSTimeout bounds the DNS builtin calls (isResolvable, dnsResolve,
	// isInNet). Zero means defaultDNSTimeout.
	DNSTimeout time.Duration
	// ScriptTimeout bounds overall script execution in Run. Zero means
	// defaultScriptTimeout.
	ScriptTimeout time.Duration
}

type PacExecutor struct {
	js            string
	program       *goja.Program
	pool          *sync.Pool
	printer       *printer.Printer
	dnsTimeout    time.Duration
	scriptTimeout time.Duration
}

func NewPac(pacJs string, p *printer.Printer, opts *PacOptions) (*PacExecutor, error) {
	js := `
(function(url,host) {
%s
return FindProxyForURL(url,host);
})(url,host)
`
	js = fmt.Sprintf(js, pacJs)
	program, err := goja.Compile("", js, false)
	if err != nil {
		return nil, stacktrace.Propagate(err, "unable to compile PAC script")
	}
	if opts == nil {
		opts = &PacOptions{}
	}
	dnsTimeout := opts.DNSTimeout
	if dnsTimeout <= 0 {
		dnsTimeout = defaultDNSTimeout
	}
	scriptTimeout := opts.ScriptTimeout
	if scriptTimeout <= 0 {
		scriptTimeout = defaultScriptTimeout
	}
	return &PacExecutor{
		js:            js,
		program:       program,
		pool:          &sync.Pool{},
		printer:       p,
		dnsTimeout:    dnsTimeout,
		scriptTimeout: scriptTimeout,
	}, nil
}

func (p *PacExecutor) Run(url, host string) (string, error) {
	// get runtime from pool or create a new one
	var runtime *goja.Runtime
	item := p.pool.Get()
	if item == nil {
		runtime = p.build()
	} else {
		runtime = item.(*goja.Runtime)
	}
	defer func() {
		runtime.ClearInterrupt()
		p.pool.Put(runtime)
	}()
	// execute code
	runtime.Set("url", url)
	runtime.Set("host", host)
	timer := time.AfterFunc(p.scriptTimeout, func() {
		runtime.Interrupt("PAC script execution timed out")
	})
	val, err := runtime.RunProgram(p.program)
	timer.Stop()
	if err != nil {
		return "", err // no wrap
	}
	return val.String(), nil
}

func (p *PacExecutor) build() *goja.Runtime {
	runtime := goja.New()
	runtime.Set("isPlainHostName", isPlainHostName)
	runtime.Set("dnsDomainIs", dnsDomainIs)
	runtime.Set("localHostOrDomainIs", localHostOrDomainIs)
	runtime.Set("isResolvable", func(host string) bool {
		return isResolvable(host, p.dnsTimeout)
	})
	runtime.Set("isInNet", func(host, pattern, mask string) bool {
		return isInNet(host, pattern, mask, p.dnsTimeout)
	})
	runtime.Set("dnsResolve", func(host string) string {
		return dnsResolve(host, p.dnsTimeout)
	})
	runtime.Set("convert_addr", convert_addr)
	runtime.Set("myIpAddress", myIpAddress)
	runtime.Set("dnsDomainLevels", dnsDomainLevels)
	runtime.Set("shExpMatch", shExpMatch)
	runtime.Set("weekdayRange", weekdayRange)
	runtime.Set("dateRange", dateRange)
	runtime.Set("timeRange", timeRange)
	runtime.Set("dnsResolveEx", func(host string) string {
		return dnsResolveEx(host, p.dnsTimeout)
	})
	runtime.Set("isResolvableEx", func(host string) bool {
		return isResolvableEx(host, p.dnsTimeout)
	})
	runtime.Set("isInNetEx", isInNetEx)
	runtime.Set("myIpAddressEx", myIpAddressEx)
	runtime.Set("alert", func(message string) {
		if p.printer != nil {
			p.printer.Infof("%s", message)
		}
	})
	return runtime
}
