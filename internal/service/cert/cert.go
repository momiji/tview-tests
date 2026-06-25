// Package cert provides the X.509 building blocks for MITM: a self-signed
// CA and per-host certificates signed by it. The Manager caches and lazily
// mints leaf certificates for inbound TLS termination.
//
// See https://shaneutt.com/blog/golang-ca-and-signed-cert-go/
package cert

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"time"
)

type Cert struct {
	Priv *rsa.PrivateKey
	Pub  *x509.Certificate
}

func NewCert(cert *x509.Certificate, bits int, ca *Cert) (*Cert, error) {
	// create our private key
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, err
	}

	// create the public key
	caPriv := priv
	caPub := cert
	if ca != nil {
		caPriv = ca.Priv
		caPub = ca.Pub
	}

	pubBytes, err := x509.CreateCertificate(rand.Reader, cert, caPub, &priv.PublicKey, caPriv)
	if err != nil {
		return nil, err
	}
	pub, err := x509.ParseCertificate(pubBytes)
	if err != nil {
		return nil, err
	}

	// return cert
	return &Cert{
		Priv: priv,
		Pub:  pub,
	}, nil
}

func NewBasicCACertConfig(cn string, serial int64) *x509.Certificate {
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore:             time.Now().Add(time.Hour * -1),
		NotAfter:              time.Now().AddDate(100, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	return cert
}

func NewBasicHttpsCertConfig(cn string, names []string, serial int64) *x509.Certificate {
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject: pkix.Name{
			CommonName: cn,
		},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(time.Hour * -1),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	if names != nil {
		for _, name := range names[:] {
			if ip := net.ParseIP(name); ip != nil {
				cert.IPAddresses = append(cert.IPAddresses, ip)
			} else {
				cert.DNSNames = append(cert.DNSNames, name)
			}
		}
	}
	return cert
}

func NewCertFromPEM(public, private string) (*Cert, error) {
	block, _ := pem.Decode([]byte(public))
	pub, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	block, _ = pem.Decode([]byte(private))
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return &Cert{
		Priv: priv,
		Pub:  pub,
	}, nil
}

func NewCertFromFiles(public, private string) (*Cert, error) {
	pub, err := os.ReadFile(public)
	if err != nil {
		return nil, err
	}
	priv, err := os.ReadFile(private)
	if err != nil {
		return nil, err
	}
	return NewCertFromPEM(string(pub), string(priv))
}

func (c *Cert) ToPEM() (string, string, error) {
	pub := new(bytes.Buffer)
	err := pem.Encode(pub, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: c.Pub.Raw,
	})
	if err != nil {
		return "", "", err
	}
	priv := new(bytes.Buffer)
	err = pem.Encode(priv, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(c.Priv),
	})
	if err != nil {
		return "", "", err
	}
	return pub.String(), priv.String(), nil
}

func (c *Cert) SaveToFiles(public, private string) error {
	pub, priv, err := c.ToPEM()
	if err != nil {
		return err
	}
	// save files
	err = os.WriteFile(public, []byte(pub), 0644)
	if err != nil {
		return err
	}
	err = os.WriteFile(private, []byte(priv), 0600)
	if err != nil {
		return err
	}
	return nil
}
