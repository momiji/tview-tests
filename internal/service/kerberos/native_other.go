//go:build !windows && !linux

package kerberos

import (
	"errors"
	"sync"

	"github.com/jcmturner/gokrb5/v8/config"

	"test/internal/service/printer"
)

var NativeKerberos = &NoKerberos{}

type NoKerberos struct {
	mutex sync.Mutex
	cfg   *config.Config
}

func (k *NoKerberos) SafeTryLogin(p *printer.Printer) error {
	return errors.New("unable to use native kerberos on this OS")
}

func (k *NoKerberos) SafeGetToken(protocol string, host string) (*string, error) {
	return nil, errors.New("unable to use native kerberos on this OS")
}
