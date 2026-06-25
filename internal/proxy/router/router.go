// Package router decides, at runtime, which rule and which upstream
// proxies a request maps to. It evaluates the configured rules (and their
// PAC scripts), caches the per-host result, and generates the local
// proxy.pac served to clients.
package router

import (
	"strconv"
	"strings"
	"sync"

	"test/internal/config"
	"test/internal/service/printer"
)

// proxyContinue is the sentinel meaning "this rule resolved to DIRECT via a
// PAC CONTINUE; keep scanning the following rules".
var proxyContinue = config.Proxy{}

type hostCache struct {
	rule  *config.Rule
	proxy []*config.Proxy
}

// Router answers match queries against a resolved configuration. It owns the
// mutable host cache (kept out of the immutable config) and the generated
// proxy.pac string.
type Router struct {
	conf            *config.ProxyConf
	pac             string
	hostsCache      map[string]*hostCache
	hostsCacheMutex sync.RWMutex
	pacsCache       map[string]string
	needFastReload  bool
	printer         *printer.Printer
}

// NewRouter builds a router for conf: it downloads the PAC scripts of the
// used pac proxies and generates the local proxy.pac. It performs network I/O
// (the PAC downloads).
func NewRouter(conf *config.ProxyConf, p *printer.Printer) (*Router, error) {
	r := &Router{
		conf:       conf,
		hostsCache: map[string]*hostCache{},
		pacsCache:  map[string]string{},
		printer:    p,
	}
	if err := r.downloadPacs(); err != nil {
		return nil, err
	}
	if err := r.genPac(); err != nil {
		return nil, err
	}
	return r, nil
}

// Pac returns the generated proxy.pac served to clients.
func (r *Router) Pac() string {
	return r.pac
}

// NeedFastReload reports whether a PAC download failed and a quick reload
// should be scheduled.
func (r *Router) NeedFastReload() bool {
	return r.needFastReload
}

func (r *Router) MatchHttp(url string, hostPort string) (*config.Rule, []*config.Proxy) {
	return r.match(url, hostPort, "http:", r.conf.Rules)
}

func (r *Router) MatchSocks(hostPort string) (*config.Rule, []*config.Proxy) {
	return r.match(hostPort, hostPort, "socks:", r.conf.SocksRules)
}

func (r *Router) match(url string, hostPort string, prefix string, rules []*config.Rule) (*config.Rule, []*config.Proxy) {
	if hc, ok := r.getCachedHost(prefix + hostPort); ok {
		return hc.rule, hc.proxy
	}
	hostOnly := strings.Split(hostPort, ":")[0]
	var direct *config.Rule
	for _, rule := range rules {
		match := false
		if rule.Regex.Pattern == nil {
			match = true
		} else if !r.conf.ExperimentalHostsCache && strings.Contains(rule.Regex.Regex, "/") {
			match = rule.Regex.Pattern.MatchString(url) != rule.Regex.Exclude
		} else if strings.Contains(rule.Regex.Regex, ":") {
			match = rule.Regex.Pattern.MatchString(hostPort) != rule.Regex.Exclude
		} else {
			match = rule.Regex.Pattern.MatchString(hostOnly) != rule.Regex.Exclude
		}
		if match {
			proxy := r.resolve(url, hostOnly, rule)
			if proxy != nil && *proxy[0] != proxyContinue {
				r.addCachedHost(prefix+hostPort, rule, proxy)
				return rule, proxy
			}
			direct = rule
		}
	}
	// if last successful rule is a pac rule which returned DIRECT, then return a "direct" proxy
	// otherwise, return nil
	if direct != nil {
		rule := direct
		proxy := []*config.Proxy{r.conf.Proxies[config.ProxyDirect.Name()]}
		r.addCachedHost(prefix+hostPort, rule, proxy)
		return rule, proxy
	}
	r.addCachedHost(prefix+hostPort, nil, nil)
	return nil, nil
}

func (r *Router) addCachedHost(hostPort string, rule *config.Rule, proxy []*config.Proxy) {
	r.hostsCacheMutex.Lock()
	defer r.hostsCacheMutex.Unlock()
	r.hostsCache[hostPort] = &hostCache{rule, proxy}
}

func (r *Router) getCachedHost(hostPort string) (*hostCache, bool) {
	r.hostsCacheMutex.RLock()
	defer r.hostsCacheMutex.RUnlock()
	if hc, ok := r.hostsCache[hostPort]; ok {
		return hc, true
	}
	return nil, false
}

func (r *Router) resolve(url, host string, rule *config.Rule) []*config.Proxy {
	proxy := r.conf.Proxies[rule.FirstProxy()]
	if proxy == nil {
		return nil
	}
	if *proxy.Type != config.ProxyPac {
		return r.allProxies(rule)
	}
	pacResult := r.resolvePac(url, host, proxy)
	switch {
	case pacResult.isDirect:
		// return continue to continue scanning rules
		// if no more rules then this will be transformed into a DIRECT (see match)
		return []*config.Proxy{&proxyContinue}
	case pacResult.isSocks, pacResult.isProxy:
		// lookup hostPort in existing proxies (host/port and pac), if found use it, otherwise create a new one
		var pacProxies []*config.Proxy
		for _, confProxy := range r.conf.PacProxies {
			var found *config.Proxy
			if strings.Contains(confProxy.PacRegex.Regex, ":") {
				if confProxy.PacRegex.Pattern.MatchString(pacResult.hostPort) {
					found = confProxy
				}
			} else if confProxy.PacRegex != nil {
				if confProxy.PacRegex.Pattern.MatchString(pacResult.hostOnly) {
					found = confProxy
				}
			}
			if found != nil {
				if *found.Host == "*" {
					name := found.Name + ">" + pacResult.hostPort
					// copy only necessary fields
					found = &config.Proxy{
						Name:       name,
						Type:       found.Type,
						TypeValue:  found.TypeValue,
						Host:       &pacResult.hostOnly,
						Port:       pacResult.portOnly,
						Verbose:    found.Verbose,
						Ssl:        found.Ssl,
						Spn:        found.Spn,
						Realm:      found.Realm,
						Credential: found.Credential,
						Cred:       found.Cred,
					}
				}
				pacProxies = append(pacProxies, found)
			}
		}
		if pacProxies != nil {
			return pacProxies
		}
		// otherwise create a temporary proxy
		proxyName := pacResult.proxy
		proxyType := config.ProxyAnonymous
		if pacResult.isSocks {
			proxyType = config.ProxySocks
		}
		h, p := splitHostPort(pacResult.hostPort, "127.0.0.1", "8080", false)
		port, _ := strconv.Atoi(p)
		return []*config.Proxy{{
			Name:      proxyName,
			Type:      &proxyType,
			TypeValue: proxyType.Value(),
			Host:      &h,
			Port:      port,
			Verbose:   rule.Verbose,
		}}
	}
	if pacResult.isDirect {
		return []*config.Proxy{&proxyContinue}
	}

	return []*config.Proxy{proxy}
}

func (r *Router) resolvePac(url, host string, proxy *config.Proxy) *pacResult {
	runtime := proxy.PacRuntime
	if runtime == nil {
		return &pacResult{
			proxy:    config.ProxyNone.Name(),
			isDirect: false,
			isProxy:  false,
			isSocks:  false,
			hostPort: "",
			hostOnly: "",
			portOnly: 0,
		}
	}
	proxies, _ := runtime.Run(url, host)
	firstProxy := strings.TrimSpace(strings.Split(strings.TrimSpace(proxies), ";")[0])
	split := strings.SplitN(firstProxy+" ", " ", 2)
	pType := split[0]
	pHostPort := ""
	if len(split) > 1 {
		pHostPort = strings.TrimSpace(split[1])
	}
	split = strings.Split(pHostPort, ":")
	pHostOnly := split[0]
	pPortOnly := 8080
	if len(split) > 1 {
		pPortOnly, _ = strconv.Atoi(split[1])
	}
	return &pacResult{
		proxy:    firstProxy,
		isDirect: pType == "DIRECT",
		isProxy:  pType == "PROXY" || pType == "HTTP" || pType == "HTTPS",
		isSocks:  pType == "SOCKS" || pType == "SOCKS4" || pType == "SOCKS5",
		hostPort: pHostPort,
		hostOnly: pHostOnly,
		portOnly: pPortOnly,
	}
}

func (r *Router) allProxies(rule *config.Rule) []*config.Proxy {
	allProxies := rule.AllProxiesName()
	proxies := make([]*config.Proxy, len(allProxies))
	for i, p := range allProxies {
		proxies[i] = r.conf.Proxies[p]
	}
	return proxies
}

type pacResult struct {
	proxy    string
	isDirect bool
	isProxy  bool
	isSocks  bool
	hostPort string
	hostOnly string
	portOnly int
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
