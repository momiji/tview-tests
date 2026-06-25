// Package kerberos authenticates upstream proxies with SPNEGO/Negotiate
// tokens. It keeps a store of logged-in clients keyed by credentials, and
// also exposes the native OS Kerberos implementation (ccache on Linux,
// SSPI on Windows) for password-less, single-sign-on authentication.
package kerberos

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jcmturner/gokrb5/v8/client"
	"github.com/jcmturner/gokrb5/v8/config"
	"github.com/palantir/stacktrace"

	appconfig "test/internal/config"
	"test/internal/service/printer"
)

// KDCTestTimeout bounds, in seconds, how long an exploded KDC list is cached
// before re-probing reachability.
const KDCTestTimeout = 10

// DefaultDomain is appended to a realm that has no dot, and used to expand a
// command-line domain. DefaultKrb5 is the built-in krb5 configuration used
// when none is provided. Both are vars so a build can override them.
var DefaultDomain = ".EXAMPLE.COM"

var DefaultKrb5 = `
[libdefaults]
dns_lookup_kdc = true
dns_lookup_realm = true
permitted_enctypes = sha1WithRSAEncryption-CmsOID rc2CBC-EnvOID rsaEncryption-EnvOID rsaES-OAEP-ENV-OID aes128-cts-hmac-sha1-96 aes256-cts-hmac-sha1-96 aes128-cts-hmac-sha256-128 aes256-cts-hmac-sha384-192 camellia256-cts-cmac aes256-cts-hmac-sha1-96
# force TCP instead of UDP, timeout for KDC with a small value and max retries per kdc to 1
udp_preference_limit = 1
max_retries = 1
kdc_timeout = 3000
`

type Kerberos struct {
	conf         *appconfig.ProxyConf
	printer      *printer.Printer
	krbCfg       *config.Config // all calls to NewWithPassword use a copy of this
	explodedKdcs map[string]*Kdc
	explodeMutex sync.Mutex
}

type Kdc struct {
	kdcs []string
	next time.Time
}

func NewKerberos(conf *appconfig.ProxyConf, p *printer.Printer) *Kerberos {
	return &Kerberos{
		conf:         conf,
		printer:      p,
		explodedKdcs: make(map[string]*Kdc),
	}
}

func (k *Kerberos) init() error {
	krb5 := k.conf.Krb5
	if krb5 == "" {
		krb5 = DefaultKrb5
	}
	krbCfg, err := config.NewFromString(krb5)
	if err != nil {
		return stacktrace.Propagate(err, "Kerberos error, unable to create config")
	}
	// fix KDC list by extending KDC list with server ip, when it contains alpha characters
	for i, realm := range krbCfg.Realms {
		// backup KDC
		realm.KPasswdServer = realm.KDC
		// update
		krbCfg.Realms[i] = realm
	}
	k.krbCfg = krbCfg
	return nil
}

func (k *Kerberos) explodeKdcs(realmKdcs []string) []string {
	k.explodeMutex.Lock()
	defer k.explodeMutex.Unlock()
	key := fmt.Sprintf("%v", realmKdcs)
	val := k.explodedKdcs[key]
	if val != nil {
		if len(val.kdcs) > 0 || time.Now().Before(val.next) {
			return val.kdcs
		}
	}
	newKdcs := make([]string, 0)
	for _, kdcs := range realmKdcs {
		for _, kdc := range strings.Split(kdcs, " ") {
			kdc = strings.TrimSpace(kdc)
			if strings.ContainsAny(strings.ToLower(kdc), "abcdefghijklmnopqrstuvwxyz") {
				host, port := splitHostPort(kdc, "127.0.0.1", "88", false)
				ips, err := net.LookupHost(host)
				if err != nil {
					newKdcs = append(newKdcs, host+":"+port)
				} else {
					for _, ip := range ips {
						newKdcs = append(newKdcs, ip+":"+port)
					}
				}
			} else {
				host, port := splitHostPort(kdc, "127.0.0.1", "88", false)
				newKdcs = append(newKdcs, host+":"+port)
			}
		}
	}
	// check if any kdcs can be reached over the network
	reachable := false
	for _, kdc := range newKdcs {
		if k.testConn(kdc) {
			reachable = true
			break
		}
	}
	// else, just empty the kdcs list so it can be checked later
	if !reachable {
		newKdcs = []string{}
	}
	// cache result
	k.explodedKdcs[key] = &Kdc{
		kdcs: newKdcs,
		next: time.Now().Add(KDCTestTimeout * time.Second),
	}
	// return
	return newKdcs
}

func (k *Kerberos) testConn(hostPort string) bool {
	dialer := new(net.Dialer)
	dialer.Timeout = time.Duration(k.conf.ConnectTimeout) * time.Second
	checkConn, err := dialer.Dial("tcp4", hostPort)
	if err != nil {
		return false
	}
	_ = checkConn.Close()
	return true
}

func (k *Kerberos) NewWithPassword(username, realm, password string) *client.Client {
	// work on a copy of krbCfg
	krbCfg := &(*k.krbCfg)
	// derive realm from username if present
	username, realm = splitUsername(username, realm)
	if k.conf.Domains[realm] != nil {
		realm = *k.conf.Domains[realm]
	} else if !strings.Contains(realm, ".") {
		// if no dot, append default domain
		realm = realm + DefaultDomain
	}

	// set default domain, which is required to be good for krb5 library to work (bug?)
	krbCfg.LibDefaults.DefaultRealm = realm
	// inject realm with default kdc equals to realm name
	var foundRealm *config.Realm
	for _, r := range krbCfg.Realms {
		if r.Realm == realm {
			foundRealm = &r
			break
		}
	}
	if foundRealm == nil {
		// work on a copy of krbCfg
		newRealm := config.Realm{
			Realm:         realm,
			KPasswdServer: []string{realm + ":88"},
		}
		// also explode kdc to all its known ips, allowing to find a working IP (firewall restriction)
		// unfortunately, this is not working with cross-domain calls, as all domains must be defined but are not known
		// newRealm.KDC = k.explodeKdcs(newRealm.KDC)
		krbCfg.Realms = append(krbCfg.Realms, newRealm)
		foundRealm = &newRealm
	}
	// if no kdcs, do not create client
	if len(foundRealm.KDC) == 0 {
		foundRealm.KDC = k.explodeKdcs(foundRealm.KPasswdServer)
		if len(foundRealm.KDC) == 0 {
			return nil
		}
	}
	// create new client
	k.printer.Infof("[-] Authenticating user '%s' on realm '%s'", username, realm)
	cl := client.NewWithPassword(username, realm, password, krbCfg, client.DisablePAFXFAST(true))
	return cl
}

// splitUsername extracts a realm embedded in a username (DOMAIN\user or
// user@domain) and uppercases it.
func splitUsername(username, realm string) (string, string) {
	if strings.Contains(username, `\`) {
		p := strings.LastIndex(username, `\`)
		realm = username[:p]
		username = username[p+1:]
	} else if strings.Contains(username, "@") {
		p := strings.LastIndex(username, "@")
		realm = username[p+1:]
		username = username[:p]
	}
	realm = strings.ToUpper(realm)
	return username, realm
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
