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
	runtime.Set("isPlainHostName", p.isPlainHostName)
	runtime.Set("dnsDomainIs", p.dnsDomainIs)
	runtime.Set("localHostOrDomainIs", p.localHostOrDomainIs)
	runtime.Set("isResolvable", p.isResolvable)
	runtime.Set("isInNet", p.isInNet)
	runtime.Set("dnsResolve", p.dnsResolve)
	runtime.Set("convert_addr", p.convert_addr)
	runtime.Set("myIpAddress", p.myIpAddress)
	runtime.Set("dnsDomainLevels", p.dnsDomainLevels)
	runtime.Set("shExpMatch", p.shExpMatch)
	runtime.Set("weekdayRange", p.weekdayRange)
	runtime.Set("dateRange", p.dateRange)
	runtime.Set("timeRange", p.timeRange)
	runtime.Set("dnsResolveEx", p.dnsResolveEx)
	runtime.Set("isResolvableEx", p.isResolvableEx)
	runtime.Set("isInNetEx", p.isInNetEx)
	runtime.Set("myIpAddressEx", p.myIpAddressEx)
	runtime.Set("alert", p.alert)
	return runtime
}

func (p *PacExecutor) alert(message string) {
	if p.printer != nil {
		p.printer.Infof("%s", message)
	}
}
