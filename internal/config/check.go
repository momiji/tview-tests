package config

import (
	"strings"

	"github.com/palantir/stacktrace"
)

func check(fc *FileConfig) error {
	// check proxies
	for name, proxy := range fc.Proxies {
		if name == "" || name == ProxyDirect.Name() || name == ProxyNone.Name() || strings.HasPrefix(name, "$") {
			return stacktrace.NewError("proxy '%s': name cannot be empty, 'direct', 'none' or start with a '$'", name)
		}
		if proxy.Type == nil {
			return stacktrace.NewError("proxy '%s': must contain 'type' (kerberos,socks,basic,anonymous,pac)", name)
		}
		if proxy.Type.Value() == -1 {
			return stacktrace.NewError("proxy '%s': must contain 'type' (kerberos,socks,basic,anonymous,pac)", name)
		}
		if *proxy.Type != ProxyPac {
			if proxy.Url != nil {
				return stacktrace.NewError("proxy '%s': non-pac proxy must not contain 'url'", name)
			}
			if proxy.Host == nil {
				return stacktrace.NewError("proxy '%s': non-pac proxy must contain 'host'", name)
			}
			if proxy.Port == 0 && *proxy.Host != "*" {
				return stacktrace.NewError("proxy '%s': non-pac proxy port number must be > 0", name)
			}
			if proxy.Credentials != nil {
				return stacktrace.NewError("proxy '%s': non-pac proxy must not contain 'credentials'", name)
			}
		} else {
			if proxy.Url == nil {
				return stacktrace.NewError("proxy '%s': pac proxy must contain 'url'", name)
			}
			if proxy.Host != nil {
				return stacktrace.NewError("proxy '%s': pac proxy must not contain 'host'", name)
			}
			if proxy.Port != 0 {
				return stacktrace.NewError("proxy '%s': pac proxy port number must be > 0", name)
			}
		}
		if *proxy.Type == ProxyAnonymous || *proxy.Type == ProxyPac {
			if proxy.Credential != nil {
				return stacktrace.NewError("proxy '%s': anonymous and pac proxies must not contain 'credential'", name)
			}
		}
		if proxy.Credential != nil && *proxy.Credential != "" && *proxy.Credential != CredentialKerberos && fc.Credentials[*proxy.Credential] == nil {
			return stacktrace.NewError("proxy '%s': credential '%s' must exist in 'credentials'", name, *proxy.Credential)
		}
		for _, cred := range splitCredentials(proxy.Credentials) {
			if cred != CredentialKerberos && fc.Credentials[cred] == nil {
				return stacktrace.NewError("proxy '%s': credential '%s' must exist in 'credentials'", name, cred)
			}
		}
	}
	// check credentials
	for name, cred := range fc.Credentials {
		if name == "" || name == CredentialKerberos || strings.HasPrefix(name, "$") {
			return stacktrace.NewError("credential '%s': name cannot be empty, 'kerberos' or start with '$'", name)
		}
		if cred.Password != nil && cred.Login == nil {
			return stacktrace.NewError("credential '%s': password cannot be set without login being set", name)
		}
	}
	// check http rules
	for i, rule := range fc.Rules {
		if rule.Host == nil {
			return stacktrace.NewError("rule %d: must contain 'host'", i)
		}
		if rule.Proxy == nil && rule.Dns == nil {
			return stacktrace.NewError("rule %d: must contain 'proxy' or 'dns'", i)
		}
		if rule.Proxy != nil {
			for _, p := range rule.allProxiesName() {
				if p != ProxyDirect.Name() && p != ProxyNone.Name() && fc.Proxies[p] == nil {
					return stacktrace.NewError("rule %d: '%s' must exist in 'proxies', or be 'direct' or 'none'", i, p)
				}
			}
		}
		if rule.Proxy != nil && rule.Dns != nil {
			if *rule.Proxy == ProxyDirect.Name() {
			} else if fc.Proxies[*rule.Proxy] != nil {
				for _, p := range rule.allProxiesName() {
					if *fc.Proxies[p].Type != ProxySocks {
						return stacktrace.NewError("rule %d: rule with dns must have a 'direct' proxy or proxy of type 'socks'", i)
					}
				}
			}
		}
		if rule.Dns != nil {
			hp := strings.Split(*rule.Dns, ":")
			if len(hp) == 0 || len(hp) > 2 {
				return stacktrace.NewError("rule %d: dns must be like '[IP][:PORT]', i.e 'IP' or 'IP:PORT' or ':PORT'", i)
			}
		}
	}

	// check socks rules
	for i, rule := range fc.SocksRules {
		if rule.Host == nil {
			return stacktrace.NewError("socks rule %d: must contain 'host'", i)
		}
		if rule.Proxy == nil && rule.Dns == nil {
			return stacktrace.NewError("socks rule %d: must contain 'proxy' or 'dns'", i)
		}
		if rule.Proxy != nil {
			for _, p := range rule.allProxiesName() {
				if p != ProxyDirect.Name() && p != ProxyNone.Name() && fc.Proxies[p] == nil {
					return stacktrace.NewError("socks rule %d: '%s' must exist in 'proxies', or be 'direct' or 'none'", i, p)
				}
			}
		}
		if rule.Proxy != nil {
			if *rule.Proxy == ProxyDirect.Name() {
			} else if fc.Proxies[*rule.Proxy] != nil {
				for _, p := range rule.allProxiesName() {
					if *fc.Proxies[p].Type != ProxySocks {
						return stacktrace.NewError("socks rule %d: must have a 'direct' proxy or proxy of type 'socks'", i)
					} else if fc.Proxies[p].Credential != nil && *fc.Proxies[p].Credential == "" {
						return stacktrace.NewError("socks rule %d: must not have a per-user credential (empty value)", i)
					}
				}
			}
		}
		if rule.Proxy != nil && rule.Dns != nil {
			if *rule.Proxy == ProxyDirect.Name() {
			} else if fc.Proxies[*rule.Proxy] != nil {
				for _, p := range rule.allProxiesName() {
					if *fc.Proxies[p].Type != ProxySocks {
						return stacktrace.NewError("socks rule %d: rule with dns must have a 'direct' proxy or proxy of type 'socks'", i)
					}
				}
			}
		}
		if rule.Dns != nil {
			hp := strings.Split(*rule.Dns, ":")
			if len(hp) == 0 || len(hp) > 2 {
				return stacktrace.NewError("socks rule %d: dns must be like '[IP][:PORT]', i.e 'IP' or 'IP:PORT' or ':PORT'", i)
			}
		}
	}

	return nil
}
