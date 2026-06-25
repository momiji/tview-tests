package pac

import (
	"fmt"
	"sync"

	"github.com/dop251/goja"
	"github.com/palantir/stacktrace"

	"test/internal/service/printer"
)

type PacExecutor struct {
	js      string
	program *goja.Program
	pool    *sync.Pool
	printer *printer.Printer
}

func NewPac(pacJs string, p *printer.Printer) (*PacExecutor, error) {
	js := `
(function(url,host) {
%s
return FindProxyForURL(url,host);
})(url,host)
`
	js = fmt.Sprintf(js, pacJs)
	program, err := goja.Compile("", js, false)
	if err != nil {
		return nil, stacktrace.Propagate(err, "unable to compile regex")
	}
	return &PacExecutor{
		js:      js,
		program: program,
		pool:    &sync.Pool{},
		printer: p,
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
	defer p.pool.Put(runtime)
	// execute code
	runtime.Set("url", url)
	runtime.Set("host", host)
	val, err := runtime.RunProgram(p.program)
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
	runtime.Set("isResolvable", isResolvable)
	runtime.Set("isInNet", isInNet)
	runtime.Set("dnsResolve", dnsResolve)
	runtime.Set("convert_addr", convert_addr)
	runtime.Set("myIpAddress", myIpAddress)
	runtime.Set("dnsDomainLevels", dnsDomainLevels)
	runtime.Set("shExpMatch", shExpMatch)
	runtime.Set("weekdayRange", weekdayRange)
	runtime.Set("dateRange", dateRange)
	runtime.Set("timeRange", timeRange)
	runtime.Set("alert", func(message string) {
		p.printer.Infof("%s", message)
	})
	return runtime
}
