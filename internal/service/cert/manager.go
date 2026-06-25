package cert

import (
	"crypto/tls"
	"strings"
	"sync"
	"time"
)

// Manager caches per-host leaf certificates signed by a CA, minting them on
// demand. Names preloaded with a "*." prefix get a wildcard cert eagerly;
// "**"/"**." entries are minted lazily on first request matching them.
type Manager struct {
	lock         sync.RWMutex
	prefix       string
	ca           *Cert
	certificates map[string]*tls.Certificate
	lastMicro    int64
}

func NewManager(ca *Cert, prefix string, names []string) (*Manager, error) {
	certs := Manager{
		prefix:       prefix,
		ca:           ca,
		certificates: make(map[string]*tls.Certificate),
	}
	// preload initial certificates
	var err error
	for _, dns := range names {
		if strings.HasPrefix(dns, "*.") {
			certs.certificates[dns], err = certs.newCertificate(dns)
			if err != nil {
				return nil, err
			}
		} else if strings.HasPrefix(dns, "**.") {
			certs.certificates[dns] = nil
		} else if dns == "**" {
			certs.certificates[dns] = nil
		} else {
			certs.certificates[dns], err = certs.newCertificate(dns)
		}
	}
	return &certs, nil
}

func (c *Manager) GetCertificate(dns string) (*tls.Certificate, error) {
	c.lock.RLock()
	cert, err := c.findCertificate(dns, false)
	c.lock.RUnlock()
	if err != nil || cert != nil {
		return cert, err
	}
	// second run, with write lock
	c.lock.Lock()
	defer c.lock.Unlock()
	//
	return c.findCertificate(dns, true)
}

func (c *Manager) newCertificate(dns string) (*tls.Certificate, error) {
	// create new cert with new dns names
	newMicro := time.Now().UnixMicro()
	if newMicro <= c.lastMicro {
		newMicro = c.lastMicro + 1
	}
	c.lastMicro = newMicro
	server, err := NewCert(NewBasicHttpsCertConfig(c.prefix+dns, []string{dns}, c.lastMicro), 2048, c.ca)
	if err != nil {
		return nil, err
	}
	pub, priv, err := server.ToPEM()
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair([]byte(pub), []byte(priv))
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

func (c *Manager) findCertificate(dns string, lock bool) (*tls.Certificate, error) {
	// find x.y.z
	if cert, ok := c.certificates[dns]; ok {
		return cert, nil
	}
	// find *.y.z
	domain := ""
	split := strings.Split(dns, ".")
	if len(split) > 1 {
		split[0] = "*"
		domain = strings.Join(split, ".")
		if cert, ok := c.certificates[domain]; ok {
			if lock {
				// mark dns as known to prevent next search with *.
				c.certificates[dns] = cert
				return cert, nil
			}
			// need a second run
			return nil, nil
		}
	}
	// find **.y.z
	for i := 0; i < len(split); i++ {
		split[i] = "**"
		domains := strings.Join(split[i:], ".")
		if _, ok := c.certificates[domains]; ok {
			if lock {
				name := domain
				if domain == "" {
					name = dns
				}
				cert, err := c.newCertificate(name)
				if err != nil {
					return nil, err
				}
				// mark dns x.y.z as known to prevent next search with *.
				c.certificates[dns] = cert
				// mark dns *.y.z as known to prevent next search with **.
				if domain != "" {
					c.certificates[domain] = cert
				}
				// return cert
				return cert, nil
			}
			// need a second run
			return nil, nil
		}

	}
	return nil, nil
}
