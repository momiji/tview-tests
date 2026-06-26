// Package processor handles a single proxied connection: it reads the
// request, matches it to a rule and upstream proxy, authenticates, and
// forwards traffic. The work is split by axes (MIGRATION.md §3): a connector
// (direct / http-forward / socks) establishes the upstream connection, an
// authenticator provides the credentials, and a transport brick (http /
// tunnel / mitm) moves the bytes.
package processor

import (
	"context"
	"sync/atomic"

	"test/internal/config"
	"test/internal/proxy/router"
	"test/internal/proxy/upstream"
	"test/internal/service/cert"
	"test/internal/service/kerberos"
	"test/internal/service/printer"
)

// snapshot is the hot-reloadable part of the runtime: the resolved config and
// the components derived from it. It is published atomically so a reload
// swaps a whole new snapshot at once, and each Process captures the snapshot
// it started with.
type snapshot struct {
	conf     *config.ProxyConf
	router   *router.Router
	selector *upstream.Selector
}

// Runtime bundles the shared services a Process needs and carries the
// shutdown context. It replaces the old Proxy's runtime aspects (the
// listeners live in the server package). Shutdown is by context cancellation
// instead of os.Exit. The config/router/selector are swapped atomically on
// reload; the kerberos store, cert manager and printer are stable.
type Runtime struct {
	current  atomic.Pointer[snapshot]
	kerberos *kerberos.Store
	certs    *cert.Manager
	printer  *printer.Printer
	traffic  TrafficSink

	ctx          context.Context
	cancel       context.CancelFunc
	newRequestId atomic.Int32
	loadCounter  atomic.Int32
}

func NewRuntime(ctx context.Context, conf *config.ProxyConf, rt *router.Router, sel *upstream.Selector, krb *kerberos.Store, certs *cert.Manager, p *printer.Printer) *Runtime {
	ctx, cancel := context.WithCancel(ctx)
	r := &Runtime{
		kerberos: krb,
		certs:    certs,
		printer:  p,
		traffic:  nopSink{},
		ctx:      ctx,
		cancel:   cancel,
	}
	r.current.Store(&snapshot{conf: conf, router: rt, selector: sel})
	return r
}

// SetTrafficSink installs the traffic sink (the UI's table adapter). Call it
// before serving; the default is a no-op sink.
func (r *Runtime) SetTrafficSink(sink TrafficSink) { r.traffic = sink }

// Reload publishes a new config/router/selector snapshot and bumps the load
// counter, so connections started before the reload are not reused after it.
func (r *Runtime) Reload(conf *config.ProxyConf, rt *router.Router, sel *upstream.Selector) {
	r.current.Store(&snapshot{conf: conf, router: rt, selector: sel})
	r.loadCounter.Add(1)
}

// Conf returns the current resolved config (the latest published snapshot).
func (r *Runtime) Conf() *config.ProxyConf { return r.current.Load().conf }

// Router returns the current router (latest published snapshot).
func (r *Runtime) Router() *router.Router { return r.current.Load().router }

// Context returns the runtime's shutdown context; the server uses it so that
// Stop() (and the parent context) tears the listeners down too.
func (r *Runtime) Context() context.Context { return r.ctx }

// Certs exposes the certificate manager (may be nil when no rule uses MITM).
func (r *Runtime) Certs() *cert.Manager { return r.certs }

// LoadCounter returns the current config generation; it changes on reload so
// in-flight connections can decide not to be reused across a reload.
func (r *Runtime) LoadCounter() int32 { return r.loadCounter.Load() }

func (r *Runtime) newReqId() int32 { return r.newRequestId.Add(1) }

func (r *Runtime) stopped() bool {
	return r.ctx.Err() != nil
}

// Stop cancels the runtime context, signaling shutdown.
func (r *Runtime) Stop() { r.cancel() }

// generateKerberosNegotiate returns a Negotiate header from a password-based
// ticket, using a cached client per realm/username/password.
func (r *Runtime) generateKerberosNegotiate(username, realm, password, protocol, host string) (*string, error) {
	if r.stopped() {
		return nil, nil
	}
	token, err := r.kerberos.SafeGetToken(username, realm, password, protocol, host)
	if err != nil {
		return nil, err
	}
	auth := "Negotiate " + *token
	return &auth, nil
}

// generateKerberosNative returns a Negotiate header from the native OS
// Kerberos implementation.
func (r *Runtime) generateKerberosNative(protocol, host string) (*string, error) {
	if r.stopped() {
		return nil, nil
	}
	token, err := kerberos.NativeKerberos.SafeGetToken(protocol, host)
	if err != nil {
		return nil, err
	}
	auth := "Negotiate " + *token
	return &auth, nil
}
