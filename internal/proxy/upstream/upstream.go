// Package upstream selects which upstream proxy (and which of its hosts) to
// use for a request, providing failover and high availability: it probes the
// candidate proxies/hosts, remembers the last reachable one, and prefers it
// next time. The "last reachable" state lives here, kept out of the immutable
// configuration.
package upstream

import (
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"test/internal/config"
	"test/internal/proxy/transport"
	"test/internal/service/printer"
)

// Selector probes and ranks the candidate upstream proxies for a request and
// caches the last reachable proxy/host so it is tried first next time.
type Selector struct {
	conf        *config.ProxyConf
	lastProxies map[string]time.Time
	lastMutex   sync.RWMutex
	printer     *printer.Printer
}

func NewSelector(conf *config.ProxyConf, p *printer.Printer) *Selector {
	return &Selector{
		conf:        conf,
		lastProxies: map[string]time.Time{},
		printer:     p,
	}
}

// FindFirstProxy returns the first reachable proxy among proxies (and the
// host:port to use), preferring the most recently reachable one. direct/none
// proxies are returned immediately without probing.
func (s *Selector) FindFirstProxy(rule *config.Rule, proxies []*config.Proxy) (*config.Proxy, string) {
	var firstProxy *config.Proxy
	var firstHostPort string
	sortedProxies := append(proxies[:0], proxies...)
	if sortedProxies != nil {
		// sort proxies
		if len(sortedProxies) > 1 {
			s.lastMutex.RLock()
			sort.SliceStable(sortedProxies, func(i int, j int) bool {
				l1 := s.lastProxies[sortedProxies[i].Name]
				l2 := s.lastProxies[sortedProxies[j].Name]
				return l1.After(l2)
			})
			s.lastMutex.RUnlock()
		}
		// find first working proxy
	proxyLoop:
		for pi, proxy := range sortedProxies {
			if *proxy.Type == config.ProxyDirect || *proxy.Type == config.ProxyNone {
				firstProxy = proxy
				break
			}
			// get hosts and port
			hosts := []string{""}
			if proxy.Host != nil {
				hosts = strings.Split(*proxy.Host, ",")
			}
			port := proxy.Port
			// sort hosts
			if len(hosts) > 1 {
				s.lastMutex.RLock()
				sort.SliceStable(hosts, func(i int, j int) bool {
					l1 := s.lastProxies[proxy.Name+"."+hosts[i]]
					l2 := s.lastProxies[proxy.Name+"."+hosts[j]]
					return l1.After(l2)
				})
				s.lastMutex.RUnlock()
			}
			// loop on hosts
			for hi, host := range hosts {
				hostPort := net.JoinHostPort(host, strconv.Itoa(port))
				// set default proxy
				if firstProxy == nil {
					firstProxy = proxy
					firstHostPort = hostPort
				}
				// try to connect to host
				dialer := new(net.Dialer)
				dialer.Timeout = time.Duration(s.conf.ConnectTimeout) * time.Second
				checkConn, err := dialer.Dial("tcp4", hostPort)
				if err != nil {
					// on failure, try next host
					if s.conf.Debug {
						s.printer.Infof("[%s] Host %s: %v", proxy.Name, hostPort, err)
					}
					continue
				}
				transport.ConfigureConn(checkConn)
				_ = checkConn.Close()
				// update last proxy and host usage
				s.lastMutex.RLock()
				pl := s.lastProxies[proxy.Name]
				hl := s.lastProxies[proxy.Name+"."+host]
				s.lastMutex.RUnlock()
				// update last proxy usage, this is very rare
				if pi > 0 || pl.IsZero() || hi > 0 || hl.IsZero() {
					s.lastMutex.Lock()
					pl = s.lastProxies[proxy.Name]
					hl = s.lastProxies[proxy.Name+"."+host]
					if pi > 0 || pl.IsZero() || hi > 0 || hl.IsZero() {
						s.lastProxies[proxy.Name] = time.Now()
						s.lastProxies[proxy.Name+"."+host] = time.Now()
					}
					s.lastMutex.Unlock()
				}
				if s.conf.Debug {
					if pi > 0 || (pl.IsZero() && len(sortedProxies) > 1) {
						s.printer.Infof("[%s] Now using proxy %s", proxyShortName(*rule.Proxy), proxy.Name)
					}
					if hi > 0 || (hl.IsZero() && len(hosts) > 1) {
						s.printer.Infof("[%s] Now using host %s", proxy.Name, host)
					}
				}
				// set firstProxy
				firstProxy = proxy
				firstHostPort = hostPort
				break proxyLoop
			}
		}
	}
	return firstProxy, firstHostPort
}

func proxyShortName(s string) string {
	if strings.Contains(s, ",") {
		return strings.Split(s, ",")[0] + "+"
	}
	return s
}
