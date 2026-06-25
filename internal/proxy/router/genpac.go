package router

import (
	"fmt"
	"strings"

	"test/internal/config"
)

// genPac generates the local proxy.pac served to clients, combining the
// configured rules and the (already downloaded) PAC scripts into a single
// FindProxyForURL function.
func (r *Router) genPac() error {
	builder := strings.Builder{}
	builder.WriteString(`
var FindProxyForURL = function(profiles) {
  return function(url, host) {
    "use strict";
    var index = 0, result = null, direct = null;
    do {
      result = profiles[index++];
      if (typeof result === "function") {
        result = result(url, host);
        if (result === "CONTINUE") { direct = result; result = null; }
      }
    } while (typeof result !== "string" && index < profiles.length);
    if (result != null) return result;
    if (direct != null) return "DIRECT";
    return "PROXY 127.0.0.1:1";
  }
}([`)
	// proxy loop
	fn := false
	startFn := func() {
		builder.WriteString(`
function(url, host) {
  "use strict";
`)
		fn = true
	}
	endFn := func() {
		builder.WriteString(`  return null;
},`)
		fn = false
	}
	for _, rule := range r.conf.Rules {
		switch {
		case rule.Dns != nil:
		case rule.Proxy == nil:
			x := ""
			if rule.Regex.Exclude {
				x = "!"
			}
			p := "PROXY 127.0.0.1:1"
			if !fn {
				startFn()
			}
			builder.WriteString(fmt.Sprint("  if (", x, "/", rule.Regex.Regex, `/.test(host)) return "`, p, `";`, "\n"))
		case *r.conf.Proxies[rule.FirstProxy()].Type == config.ProxyPac:
			x := ""
			if !rule.Regex.Exclude { // inverse exclude as if condition is true then return null
				x = "!"
			}
			proxy := r.conf.Proxies[rule.FirstProxy()]
			if fn {
				endFn()
			}
			builder.WriteString(`
function(url, host) {
`)
			builder.WriteString(fmt.Sprint("  if (", x, "/", rule.Regex.Regex, `/.test(host)) return null;`))
			builder.WriteString(fmt.Sprintf(`
  var f = function() {
/* Begin of PAC */
%s
/* End of PAC */
    return FindProxyForURL;
  }.call(this);
  var r = f(url, host).trim();
  if (r === "DIRECT") return "CONTINUE";
  var firstProxy = r.split(";")[0].trim();
  var split = firstProxy.split(" ");
  var type = split[0];
  var hostPort = "";
  if (split.length > 1) hostPort = split.slice(1).join(" ").trim();
  var hostOnly = hostPort.split(":")[0];
`, *proxy.PacJs))
			for _, confProxy := range r.conf.Proxies {
				if confProxy.PacRegex != nil {
					p := r.conf.PacProxy
					if confProxy.PacProxy != nil {
						p = *confProxy.PacProxy
					}
					if strings.Contains(confProxy.PacRegex.Regex, ":") {
						builder.WriteString(fmt.Sprint("  if (/", confProxy.PacRegex.Regex, `/.test(hostPort)) return "`, p, `";`, "\n"))
					} else if confProxy.PacRegex != nil {
						builder.WriteString(fmt.Sprint("  if (/", confProxy.PacRegex.Regex, `/.test(hostOnly)) return "`, p, `";`, "\n"))
					}
				}
			}
			builder.WriteString(`  return r;
},`)
		default:
			x := ""
			if rule.Regex.Exclude {
				x = "!"
			}
			p := ""
			for _, n := range rule.AllProxiesName() {
				proxy := r.conf.Proxies[n]
				if proxy.PacProxy != nil {
					p = p + ";" + *proxy.PacProxy
				}
			}
			if p == "" {
				p = r.conf.PacProxy
			} else {
				p = p[1:]
			}
			if !fn {
				startFn()
			}
			builder.WriteString(fmt.Sprint("  if (", x, "/", rule.Regex.Regex, `/.test(host)) return "`, p, `";`, "\n"))
		}
	}
	// end main function
	if fn {
		endFn()
	}
	builder.WriteString(`
null
]);
`)
	r.pac = strings.ReplaceAll(builder.String(), "\r", "")
	return nil
}
