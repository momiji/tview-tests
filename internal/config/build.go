package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/palantir/stacktrace"

	"test/internal/service/secret"
)

// featureHostsCache is the experimental feature name enabling the coarse
// per-host match cache (which disables fine-grained url matching).
const featureHostsCache = "hosts-cache"

// build turns a validated FileConfig into a resolved ProxyConf: it applies
// the tri-state cascade for the verbose/debug/trace/mitm switches, links and
// synthesizes credentials, adds the built-in proxies, decrypts passwords,
// compiles the host/pac regexes and the PAC proxy strings, and marks what is
// actually used. It does no network and no routing work (the PAC download and
// matching live in the router).
func build(args CmdArgs, fc *FileConfig) (*ProxyConf, error) {
	pc := &ProxyConf{
		Bind:           fc.Bind,
		Port:           fc.Port,
		SocksPort:      fc.SocksPort,
		ConnectTimeout: fc.ConnectTimeout,
		Proxies:        map[string]*Proxy{},
		Credentials:    map[string]*Cred{},
		Domains:        fc.Domains,
		Krb5:           fc.Krb5,
		Check:          fc.Check,
		Update:         fc.Update,
		Restart:        fc.Restart,
		UseEnvProxy:    fc.UseEnvProxy,
		Experimental:   fc.Experimental,
		ACL:            fc.ACL,
		ConsoleUI:      fc.ConsoleUI || args.ConsoleUI,
	}
	if pc.ConnectTimeout == 0 {
		pc.ConnectTimeout = DefaultConnectTimeout
	}
	// build server bind
	if pc.Bind == "" {
		pc.Bind = "127.0.0.1"
	}
	// build server pac proxy string
	pc.PacProxy = fmt.Sprint("PROXY ", pc.Bind, ":", pc.Port)
	pc.ExperimentalHostsCache = isExperimental(fc.Experimental, featureHostsCache)
	// global effective switches: args force > file global
	pc.Trace = effective(args.Trace, fc.Trace)
	pc.Debug = effective(args.Debug, fc.Debug) || pc.Trace
	pc.Verbose = effective(args.Verbose, fc.Verbose) || pc.Debug

	// resolved credentials from file
	for name, fcred := range fc.Credentials {
		pc.Credentials[name] = &Cred{
			Name:     name,
			Login:    fcred.Login,
			Password: fcred.Password,
		}
	}
	// add none proxy
	noneName := ProxyNone.Name()
	noneType := ProxyNone
	pc.Proxies[noneName] = &Proxy{Name: noneName, Type: &noneType, TypeValue: ProxyNone.Value()}
	// add direct proxy
	directName := ProxyDirect.Name()
	directType := ProxyDirect
	pc.Proxies[directName] = &Proxy{Name: directName, Type: &directType, TypeValue: ProxyDirect.Value()}
	// add native kerberos credential
	pc.Credentials[CredentialKerberos] = &Cred{Name: CredentialKerberos, IsNative: true}

	// build proxies
	for name, fp := range fc.Proxies {
		p := &Proxy{
			Name:        name,
			Type:        fp.Type,
			TypeValue:   fp.Type.Value(),
			Host:        fp.Host,
			Port:        fp.Port,
			Ssl:         fp.Ssl,
			Spn:         fp.Spn,
			Realm:       fp.Realm,
			Credential:  fp.Credential,
			Credentials: fp.Credentials,
			Pac:         fp.Pac,
			PacOrder:    fp.PacOrder,
			Url:         fp.Url,
		}
		// effective switches: args force > file global > proxy
		p.Trace = effective(args.Trace, fc.Trace, fp.Trace)
		p.Debug = effective(args.Debug, fc.Debug, fp.Debug) || p.Trace
		p.Verbose = effective(args.Verbose, fc.Verbose, fp.Verbose) || p.Debug
		p.Mitm = effective(false, fc.Mitm, fp.Mitm)
		// credential resolution
		if *fp.Type == ProxyKerberos || *fp.Type == ProxyBasic || *fp.Type == ProxySocks {
			switch {
			case fp.Credential == nil:
				if *fp.Type == ProxyKerberos || *fp.Type == ProxyBasic {
					cn := fmt.Sprintf("$null-%s", name)
					p.Cred = &Cred{Name: cn, IsNull: true}
					pc.Credentials[cn] = p.Cred
				}
			case *fp.Credential == "":
				cn := fmt.Sprintf("$user-%s", name)
				p.Cred = &Cred{Name: cn, IsPerUser: true}
				pc.Credentials[cn] = p.Cred
			default:
				p.Cred = pc.Credentials[*fp.Credential]
			}
		}
		pc.Proxies[name] = p
	}

	// build per-proxy pac proxy strings and pac regexes (covers built-ins too)
	for _, proxy := range pc.Proxies {
		switch *proxy.Type {
		case ProxyKerberos, ProxyBasic:
			proxy.PacProxy = nil
			// if per user, directly proxy to target who will ask for credentials
			if proxy.Cred.IsPerUser {
				s := genProxy("PROXY", *proxy.Host, proxy.Port)
				proxy.PacProxy = &s
			}
		case ProxyDirect:
			s := "DIRECT"
			proxy.PacProxy = &s
		case ProxySocks:
			proxy.PacProxy = nil
			// if no authentication, directly proxy to target
			if proxy.Cred == nil {
				s := genProxy("SOCKS", *proxy.Host, proxy.Port)
				proxy.PacProxy = &s
			}
		case ProxyAnonymous:
			s := genProxy("PROXY", *proxy.Host, proxy.Port)
			proxy.PacProxy = &s
		}
		if proxy.Pac != nil {
			rx, err := compileRegex(*proxy.Pac)
			if err != nil {
				return nil, stacktrace.Propagate(err, "proxy '%s': unable to compile pac regex", proxy.Name)
			}
			proxy.PacRegex = rx
		}
	}

	// build pac proxies sorted by PacOrder
	pc.PacProxies = make([]*Proxy, 0)
	for _, proxy := range pc.Proxies {
		if proxy.Pac != nil {
			pc.PacProxies = append(pc.PacProxies, proxy)
		}
	}
	sort.SliceStable(pc.PacProxies, func(i, j int) bool { return pc.PacProxies[i].PacOrder < pc.PacProxies[j].PacOrder })

	// decrypt passwords
	cipher := secret.New(args.KeyFile)
	for name, cred := range pc.Credentials {
		if cred.Password != nil && strings.HasPrefix(*cred.Password, secret.Prefix) {
			password, err := cipher.Decrypt((*cred.Password)[len(secret.Prefix):])
			if err != nil {
				return nil, stacktrace.Propagate(err, "unable to decrypt '%s' password", name)
			}
			cred.Password = &password
		}
	}

	// build rules
	for _, fr := range fc.Rules {
		r, err := buildRule(args, fc, fr)
		if err != nil {
			return nil, err
		}
		pc.Rules = append(pc.Rules, r)
	}
	for _, fr := range fc.SocksRules {
		r, err := buildRule(args, fc, fr)
		if err != nil {
			return nil, err
		}
		pc.SocksRules = append(pc.SocksRules, r)
	}

	// update rules and IsUsed
	markUsed(pc, pc.Rules, directName)
	markUsed(pc, pc.SocksRules, directName)

	// a used pac proxy also uses every credential in its `credentials` list
	for _, proxy := range pc.Proxies {
		if proxy.IsUsed && *proxy.Type == ProxyPac {
			for _, cred := range splitCredentials(proxy.Credentials) {
				pc.Credentials[cred].IsUsed = true
			}
		}
	}

	return pc, nil
}

func buildRule(args CmdArgs, fc *FileConfig, fr *FileRule) (*Rule, error) {
	r := &Rule{Host: fr.Host, Proxy: fr.Proxy, Dns: fr.Dns}
	r.Trace = effective(args.Trace, fc.Trace, fr.Trace)
	r.Debug = effective(args.Debug, fc.Debug, fr.Debug) || r.Trace
	r.Verbose = effective(args.Verbose, fc.Verbose, fr.Verbose) || r.Debug
	r.Mitm = effective(false, fc.Mitm, fr.Mitm)
	rx, err := compileRegex(*fr.Host)
	if err != nil {
		return nil, stacktrace.Propagate(err, "unable to compile rule regex")
	}
	r.Regex = rx
	return r, nil
}

func markUsed(pc *ProxyConf, rules []*Rule, directName string) {
	for _, rule := range rules {
		if rule.Dns != nil && rule.Proxy == nil {
			dn := directName
			rule.Proxy = &dn
			continue
		}
		for _, p := range rule.AllProxiesName() {
			proxy := pc.Proxies[p]
			proxy.IsUsed = true
			if proxy.Cred != nil && !proxy.Cred.IsPerUser {
				proxy.Cred.IsUsed = true
			}
		}
	}
}

// effective returns the cascaded value of a tri-state switch: a command-line
// "force" wins; otherwise the first explicit (non-nil) level wins, scanning
// from the highest priority; otherwise false.
func effective(force bool, levels ...*bool) bool {
	if force {
		return true
	}
	for _, l := range levels {
		if l != nil {
			return *l
		}
	}
	return false
}

func isExperimental(conf string, name string) bool {
	return strings.Contains(" "+strings.ReplaceAll(conf, ",", " ")+" ", " "+name+" ")
}

// genProxy builds a PAC result string ("PROXY"/"SOCKS host:port", joined by
// ";" across comma-separated hosts).
func genProxy(name string, hosts string, port int) string {
	list := make([]string, 0)
	for _, host := range strings.Split(hosts, ",") {
		list = append(list, fmt.Sprintf("%s %s:%d", name, host, port))
	}
	return strings.Join(list, ";")
}

// compileRegex translates a host pattern into a Regex: a leading "!" negates,
// a "re:" prefix is a raw regexp, otherwise the glob (. * ? |) is translated.
func compileRegex(rule string) (*Regex, error) {
	exclude := false
	regex := rule
	if strings.HasPrefix(regex, "!") {
		regex = regex[1:]
		exclude = true
	}
	if strings.HasPrefix(regex, "re:") {
		regex = regex[3:]
	} else {
		regex = strings.ReplaceAll(regex, ".", `\.`)
		regex = strings.ReplaceAll(regex, "*", ".*")
		regex = strings.ReplaceAll(regex, "?", ".")
		regex = "^" + strings.ReplaceAll(regex, "|", "$|^") + "$"
	}
	pattern, err := regexp.Compile(regex)
	if err != nil {
		return nil, stacktrace.Propagate(err, "unable to compile regex")
	}
	return &Regex{
		Pattern: pattern,
		Regex:   regex,
		Exclude: exclude,
	}, nil
}
