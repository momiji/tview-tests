package router

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/palantir/stacktrace"
	"golang.org/x/text/encoding/charmap"

	"test/internal/config"
	"test/internal/service/pac"
)

// downloadPacs downloads the PAC script of each used pac proxy and compiles
// it into a runtime, falling back to a cached copy or to a static "default
// proxy" script when the download fails. It fills each pac proxy's PacJs and
// PacRuntime.
func (r *Router) downloadPacs() error {
	for _, proxy := range r.conf.Proxies {
		if proxy.IsUsed && *proxy.Type == config.ProxyPac {
			r.printer.Infof("[-] Loading proxy pac: %s", *proxy.Url)
			js, ex, err := r.downloadPac(*proxy.Url)
			if err != nil {
				ok := false
				if js, ok = r.pacsCache[*proxy.Url]; ok {
					r.printer.Infof("[-] Error: unable to download or use pac, using cached js")
					ex, _ = r.pacToExecutor(js)
				} else {
					r.printer.Errorf("[-] Error: %v", err)
				}
			}
			if js == "" {
				noneJs := fmt.Sprintf(`function FindProxyForURL() { return "%s"; }`, r.conf.PacProxy)
				proxy.PacJs = &noneJs
				proxy.PacRuntime = nil
				r.needFastReload = true
				continue
			}
			proxy.PacJs = &js
			proxy.PacRuntime = ex
			r.pacsCache[*proxy.Url] = js
		}
	}
	return nil
}

func (r *Router) downloadPac(url string) (string, *pac.PacExecutor, error) {
	// download pac
	httpClient := r.newHttpClient()
	get, err := httpClient.Get(url)
	if err != nil {
		return "", nil, errors.New(fmt.Sprintf("unable to download pac: %v", err))
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(get.Body)
	// check status code
	if get.StatusCode != 200 {
		return "", nil, errors.New(fmt.Sprintf("unable to download pac: HTTP %d", get.StatusCode))
	}
	// read all bytes
	jsb, err := io.ReadAll(get.Body)
	if err != nil {
		return "", nil, errors.New(fmt.Sprintf("unable to download pac: %v", err))
	}
	js := string(jsb)
	executor, err := r.pacToExecutor(js)
	if err != nil {
		return "", nil, err
	}
	return js, executor, nil
}

func (r *Router) pacToExecutor(js string) (*pac.PacExecutor, error) {
	// load js as unicode/utf-8
	executor, err := pac.NewPac(js, r.printer, nil)
	if err != nil {
		// load js as iso-latin-1
		jsb2, err := charmap.ISO8859_1.NewDecoder().Bytes([]byte(js))
		if err == nil {
			js = string(jsb2)
			executor, err = pac.NewPac(js, r.printer, nil)
			if err != nil {
				return nil, stacktrace.Propagate(err, "unable to create pac runtime")
			}
		}
	}
	return executor, nil
}

func (r *Router) newHttpClient() *http.Client {
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	if !r.conf.UseEnvProxy {
		httpTransport.Proxy = nil
	}
	httpClient := &http.Client{Timeout: 30 * time.Second, Transport: httpTransport}
	return httpClient
}
