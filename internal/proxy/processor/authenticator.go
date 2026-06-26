package processor

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"test/internal/config"
)

// computeAuthPerUser builds the upstream authorization from a client-provided
// Proxy-Authorization header (per-user mode, axis B). It returns whether the
// client is authenticated, an opaque context key, and a function that produces
// the actual upstream authorization on demand.
func (p *Process) computeAuthPerUser(firstProxy *config.Proxy, proxyAuthorization *string) (bool, string, func() (*string, error)) {
	var authenticated bool
	var authorizationContext string
	var authorizationFunc func() (*string, error)
	basic := strings.SplitN(*proxyAuthorization, " ", 2)
	if len(basic) == 2 {
		credentials, err := base64.StdEncoding.DecodeString(basic[1])
		if err == nil {
			userDetails := strings.SplitN(string(credentials), ":", 2)
			if len(userDetails) == 2 {
				switch {
				case *firstProxy.Type == config.ProxyKerberos:
					// no isNative check: there is no cred for per-user auth
					authorizationContext = p.hash("krb:%s/%s/%s/%s", userDetails[0], *firstProxy.Realm, userDetails[1], *firstProxy.Host)
					authorizationFunc = func(username string, realm string, password string, protocol string, host string) func() (*string, error) {
						return func() (*string, error) {
							// hide error, as this is not an unrecoverable error
							auth, err := p.runtime.generateKerberosNegotiate(username, realm, password, protocol, host)
							if err != nil {
								p.runtime.printer.Errorf("%s Failed to generate authenticate token: %v", p.logPrefix, err)
							}
							return auth, nil
						}
					}(userDetails[0], *firstProxy.Realm, userDetails[1], *firstProxy.Spn, *firstProxy.Host)
					authenticated = true
				case *firstProxy.Type == config.ProxyBasic:
					authorizationContext = p.hash("basic:%s", *proxyAuthorization)
					authorizationFunc = func(auth *string) func() (*string, error) {
						return func() (*string, error) {
							return auth, nil
						}
					}(proxyAuthorization)
					authenticated = true
				case *firstProxy.Type == config.ProxySocks:
					credentialString := string(credentials)
					authorizationContext = p.hash("socks:%s", credentialString)
					authorizationFunc = func(auth *string) func() (*string, error) {
						return func() (*string, error) {
							return auth, nil
						}
					}(&credentialString)
					authenticated = true
				}
			}
		}
	}
	return authenticated, authorizationContext, authorizationFunc
}

// computeAuthPerConf builds the upstream authorization from the configured
// credential (per-conf mode, axis B).
func (p *Process) computeAuthPerConf(firstProxy *config.Proxy) (bool, string, func() (*string, error)) {
	var authenticated bool
	var authorizationContext string
	var authorizationFunc func() (*string, error)
	switch {
	case *firstProxy.Type == config.ProxyKerberos && !firstProxy.Cred.IsNative:
		authorizationContext = p.hash("krb:%s/%s/%s/%s", *firstProxy.Cred.Login, *firstProxy.Realm, *firstProxy.Cred.Password, *firstProxy.Host)
		authorizationFunc = func(username string, realm string, password string, protocol string, host string) func() (*string, error) {
			return func() (*string, error) {
				// don't hide error, this is an unrecoverable error
				auth, err := p.runtime.generateKerberosNegotiate(username, realm, password, protocol, host)
				if err != nil {
					p.runtime.printer.Errorf("%s Failed to generate authenticate token: %v", p.logPrefix, err)
					return nil, err
				}
				return auth, nil
			}
		}(*firstProxy.Cred.Login, *firstProxy.Realm, *firstProxy.Cred.Password, *firstProxy.Spn, *firstProxy.Host)
		authenticated = true
	case *firstProxy.Type == config.ProxyKerberos && firstProxy.Cred.IsNative:
		authorizationContext = p.hash("native:%s", *firstProxy.Host)
		authorizationFunc = func(protocol string, host string) func() (*string, error) {
			return func() (*string, error) {
				// don't hide error, this is an unrecoverable error
				auth, err := p.runtime.generateKerberosNative(protocol, host)
				if err != nil {
					p.runtime.printer.Errorf("%s Failed to generate authenticate token: %v", p.logPrefix, err)
					return nil, err
				}
				return auth, nil
			}
		}(*firstProxy.Spn, *firstProxy.Host)
		authenticated = true
	case *firstProxy.Type == config.ProxyBasic:
		basic := fmt.Sprintf("%s:%s", *firstProxy.Cred.Login, *firstProxy.Cred.Password)
		basic = "Basic " + base64.StdEncoding.EncodeToString([]byte(basic))
		authorizationContext = p.hash("basic:%s", basic)
		authorizationFunc = func(auth *string) func() (*string, error) {
			return func() (*string, error) {
				return auth, nil
			}
		}(&basic)
		authenticated = true
	case *firstProxy.Type == config.ProxySocks:
		credentialString := fmt.Sprintf("%s:%s", *firstProxy.Cred.Login, *firstProxy.Cred.Password)
		authorizationContext = p.hash("socks:%s", credentialString)
		authorizationFunc = func(auth *string) func() (*string, error) {
			return func() (*string, error) {
				return auth, nil
			}
		}(&credentialString)
		authenticated = true
	}
	return authenticated, authorizationContext, authorizationFunc
}

func (p *Process) hash(format string, a ...any) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf(format, a...))))
}
