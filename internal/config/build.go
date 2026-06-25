package config

import (
	"fmt"
	"strings"

	"github.com/palantir/stacktrace"

	"test/internal/service/secret"
)

// build turns a validated FileConfig into a resolved ProxyConf: it applies
// the tri-state cascade for the verbose/debug/trace/mitm switches, links and
// synthesizes credentials, adds the built-in proxies, decrypts passwords and
// marks what is actually used. It does no network and no routing work.
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
	// global effective switches: args force > file global
	pc.Trace = effective(args.Trace, fc.Trace)
	pc.Debug = effective(args.Debug, fc.Debug) || pc.Trace
	pc.Verbose = effective(args.Verbose, fc.Verbose) || pc.Debug

	// resolved credentials from file
	for name, fcred := range fc.Credentials {
		pc.Credentials[name] = &Cred{
			name:     name,
			Login:    fcred.Login,
			Password: fcred.Password,
		}
	}
	// add none proxy
	noneName := ProxyNone.Name()
	noneType := ProxyNone
	pc.Proxies[noneName] = &Proxy{name: noneName, Type: &noneType, typeValue: ProxyNone.Value()}
	// add direct proxy
	directName := ProxyDirect.Name()
	directType := ProxyDirect
	pc.Proxies[directName] = &Proxy{name: directName, Type: &directType, typeValue: ProxyDirect.Value()}
	// add native kerberos credential
	pc.Credentials[CredentialKerberos] = &Cred{name: CredentialKerberos, isNative: true}

	// build proxies
	for name, fp := range fc.Proxies {
		p := &Proxy{
			name:        name,
			Type:        fp.Type,
			typeValue:   fp.Type.Value(),
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
					p.cred = &Cred{name: cn, isNull: true}
					pc.Credentials[cn] = p.cred
				}
			case *fp.Credential == "":
				cn := fmt.Sprintf("$user-%s", name)
				p.cred = &Cred{name: cn, isPerUser: true}
				pc.Credentials[cn] = p.cred
			default:
				p.cred = pc.Credentials[*fp.Credential]
			}
		}
		pc.Proxies[name] = p
	}

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
		pc.Rules = append(pc.Rules, buildRule(args, fc, fr))
	}
	for _, fr := range fc.SocksRules {
		pc.SocksRules = append(pc.SocksRules, buildRule(args, fc, fr))
	}

	// update rules and isUsed
	markUsed(pc, pc.Rules, directName)
	markUsed(pc, pc.SocksRules, directName)

	// a used pac proxy also uses every credential in its `credentials` list
	for _, proxy := range pc.Proxies {
		if proxy.isUsed && *proxy.Type == ProxyPac {
			for _, cred := range splitCredentials(proxy.Credentials) {
				pc.Credentials[cred].isUsed = true
			}
		}
	}

	return pc, nil
}

func buildRule(args CmdArgs, fc *FileConfig, fr *FileRule) *Rule {
	r := &Rule{Host: fr.Host, Proxy: fr.Proxy, Dns: fr.Dns}
	r.Trace = effective(args.Trace, fc.Trace, fr.Trace)
	r.Debug = effective(args.Debug, fc.Debug, fr.Debug) || r.Trace
	r.Verbose = effective(args.Verbose, fc.Verbose, fr.Verbose) || r.Debug
	r.Mitm = effective(false, fc.Mitm, fr.Mitm)
	return r
}

func markUsed(pc *ProxyConf, rules []*Rule, directName string) {
	for _, rule := range rules {
		if rule.Dns != nil && rule.Proxy == nil {
			dn := directName
			rule.Proxy = &dn
			continue
		}
		for _, p := range rule.allProxiesName() {
			proxy := pc.Proxies[p]
			proxy.isUsed = true
			if proxy.cred != nil && !proxy.cred.isPerUser {
				proxy.cred.isUsed = true
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
