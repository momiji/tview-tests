// Package app wires the pieces together and runs the proxy: it parses the
// CLI, loads and resolves the configuration, builds the services
// (kerberos, certificates, router, upstream selector), starts the listeners,
// and drives config hot-reload and self-update. Shutdown is by context
// cancellation (signals or the auto-exit timeout).
package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/howeyc/gopass"
	"github.com/jcmturner/gokrb5/v8/client"

	"test/internal/cli"
	"test/internal/config"
	"test/internal/proxy/processor"
	"test/internal/proxy/router"
	"test/internal/proxy/server"
	"test/internal/proxy/upstream"
	"test/internal/service/cert"
	"test/internal/service/kerberos"
	"test/internal/service/printer"
	"test/internal/ui/traffic"
	"test/internal/update"
)

// Main is the program entry point. It returns the process exit code.
func Main(meta cli.Meta) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	p := printer.New()
	go p.Run(ctx)

	client.MyCrossDomainPatch() // ensure the krb5 library is patched for cross-domain support

	args, action, err := cli.Parse(meta)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Print(cli.Usage(meta))
		return 1
	}
	switch action {
	case cli.ActionHelp:
		fmt.Print(cli.Help(meta))
		return 0
	case cli.ActionVersion:
		fmt.Println(cli.Version(meta))
		return 0
	case cli.ActionEncrypt:
		if err := cli.EncryptPassword(args); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	p.Printf("[-] Proxy %s started", cli.Version(meta))
	rc := 0
	if err := run(ctx, args, meta, p); err != nil {
		p.Errorf("[-] Error: %s", err)
		rc = 1
	}
	_ = p.Flush(context.Background())
	return rc
}

func run(ctx context.Context, args config.CmdArgs, meta cli.Meta, p *printer.Printer) error {
	conf, err := config.Load(args)
	if err != nil {
		return err
	}
	// ask for any missing credentials interactively
	disableAutoRestart, err := askCredentials(conf, p)
	if err != nil {
		return err
	}
	// certificates (only if a rule uses MITM)
	certs, err := genCerts(conf, meta, p)
	if err != nil {
		return err
	}
	// kerberos
	krb, err := kerberos.NewStore(conf, p)
	if err != nil {
		return err
	}
	if err := loadKerberos(conf, krb, p); err != nil {
		return err
	}
	// routing + upstream selection
	rt, err := router.NewRouter(conf, p)
	if err != nil {
		return err
	}
	sel := upstream.NewSelector(conf, p)

	runtime := processor.NewRuntime(ctx, conf, rt, sel, krb, certs, p)

	// traffic accounting (always on; the UI renders this table when enabled)
	trafficTable := traffic.NewTrafficTable()
	runtime.SetTrafficSink(traffic.NewSink(trafficTable))
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runtime.Context().Done():
				return
			case <-ticker.C:
				trafficTable.RemoveDead()
			}
		}
	}()

	// auto-exit after the timeout (single-proxy mode)
	if args.Timeout > 0 {
		p.Infof("[-] Proxy will exit automatically in %v seconds", args.Timeout)
		go func() {
			select {
			case <-runtime.Context().Done():
			case <-time.After(time.Duration(args.Timeout) * time.Second):
				runtime.Stop()
			}
		}()
	}
	// config hot-reload
	if args.Config != "" {
		rl := &reloader{args: args, runtime: runtime, p: p}
		go config.Watch(runtime.Context(), args.Config, rl.reload)
	}
	// self-update
	if !strings.HasPrefix(meta.Version, "dev") {
		go updateLoop(runtime, meta, disableAutoRestart, p)
	}

	return server.New(runtime, conf, p).Run(runtime.Context())
}

// askCredentials prompts for the login/password of any used, non-per-user,
// non-native credential that is missing them. It returns whether an
// interactive value was entered (which disables auto-restart on update).
func askCredentials(conf *config.ProxyConf, p *printer.Printer) (bool, error) {
	_ = p.Flush(context.Background())
	disableAutoRestart := false
	for _, cred := range conf.Credentials {
		if !cred.IsUsed || cred.IsPerUser || cred.IsNative {
			continue
		}
		message := fmt.Sprintf("Credential [%s] -", cred.Name)
		if cred.IsNull {
			message = fmt.Sprintf("Proxy [%s] -", strings.SplitN(cred.Name, "-", 2)[1])
		}
		if cred.Login == nil {
			fmt.Printf("[-] %s Enter login: ", message)
			var login string
			if _, err := fmt.Scanln(&login); err != nil {
				return false, fmt.Errorf("invalid empty login")
			}
			cred.Login = &login
			disableAutoRestart = true
		}
		if cred.Password == nil {
			fmt.Printf("[-] %s Enter password for user '%s': ", message, *cred.Login)
			pwdBytes, err := gopass.GetPasswdMasked()
			if err != nil {
				return false, fmt.Errorf("invalid empty password")
			}
			password := string(pwdBytes)
			cred.Password = &password
			disableAutoRestart = true
		}
	}
	return disableAutoRestart, nil
}

// genCerts builds the certificate manager when at least one rule uses MITM,
// loading or creating the CA from <name>.ca.crt / <name>.ca.key.
func genCerts(conf *config.ProxyConf, meta cli.Meta, p *printer.Printer) (*cert.Manager, error) {
	mitm := false
	for _, rule := range conf.Rules {
		if rule.Mitm {
			mitm = true
			break
		}
	}
	if !mitm {
		return nil, nil
	}
	caCert := meta.Name + ".ca.crt"
	caKey := meta.Name + ".ca.key"
	ca, err := cert.NewCertFromFiles(caCert, caKey)
	if err != nil {
		ca, err = cert.NewCert(cert.NewBasicCACertConfig(meta.Name+" ca - "+uuid.NewString(), time.Now().UnixMicro()), 2048, nil)
		if err != nil {
			return nil, fmt.Errorf("unable to generate CA certificate: %v", err)
		}
		if err = ca.SaveToFiles(caCert, caKey); err != nil {
			return nil, fmt.Errorf("unable to save CA certificate: %v", err)
		}
	}
	cm, err := cert.NewManager(ca, meta.Name+":", []string{"**"})
	if err != nil {
		return nil, fmt.Errorf("unable to create certificates manager: %v", err)
	}
	return cm, nil
}

// loadKerberos performs the initial login for every used kerberos proxy.
func loadKerberos(conf *config.ProxyConf, krb *kerberos.Store, p *printer.Printer) error {
	for _, proxy := range conf.Proxies {
		if *proxy.Type != config.ProxyKerberos || proxy.Cred == nil || !proxy.Cred.IsUsed {
			continue
		}
		if proxy.Cred.IsNative {
			if err := kerberos.NativeKerberos.SafeTryLogin(p); err != nil {
				return fmt.Errorf("unable to login to native os kerberos: %w", err)
			}
		} else {
			if _, err := krb.SafeTryLogin(*proxy.Cred.Login, *proxy.Realm, *proxy.Cred.Password, false); err != nil {
				return fmt.Errorf("unable to login to kerberos: %w", err)
			}
		}
	}
	return nil
}

type reloader struct {
	args     config.CmdArgs
	runtime  *processor.Runtime
	p        *printer.Printer
	lastMod  time.Time
	lastLoad time.Time
}

// reload re-reads the config file and, when it changed (or a fast reload is
// pending), rebuilds the router/selector and publishes them — but only if the
// new config does not require credentials that were not already entered.
func (r *reloader) reload() {
	stat, err := os.Stat(r.args.Config)
	if err != nil {
		return
	}
	if stat.ModTime() == r.lastMod &&
		time.Now().Before(r.lastLoad.Add(config.ReloadForceTimeout*time.Second)) &&
		!r.runtime.Router().NeedFastReload() {
		return
	}
	newConf, err := config.Load(r.args)
	r.lastMod = stat.ModTime()
	r.lastLoad = time.Now()
	if err != nil {
		r.p.Infof("[-] Error while reloading configuration: %s", err)
		return
	}
	// hot-reload is only possible if no new credentials are required
	oldConf := r.runtime.Conf()
	for _, cred := range newConf.Credentials {
		if old := oldConf.Credentials[cred.Name]; old != nil {
			if cred.Login == nil {
				cred.Login = old.Login
			}
			if cred.Password == nil {
				cred.Password = old.Password
			}
		}
		if cred.IsUsed && !cred.IsNative && (cred.Login == nil || cred.Password == nil) {
			r.p.Infof("[-] Could not Hot-reload the configuration as it requires new credentials")
			return
		}
	}
	rt, err := router.NewRouter(newConf, r.p)
	if err != nil {
		r.p.Infof("[-] Error while reloading configuration: %s", err)
		return
	}
	sel := upstream.NewSelector(newConf, r.p)
	r.p.Infof("[-] Hot-reload of the configuration succeeded")
	r.runtime.Reload(newConf, rt, sel)
}

func updateLoop(runtime *processor.Runtime, meta cli.Meta, disableAutoRestart bool, p *printer.Printer) {
	select {
	case <-runtime.Context().Done():
		return
	case <-time.After(3 * time.Second):
	}
	for {
		restart := update.Update(runtime.Conf(), update.Meta{Name: meta.Name, Version: meta.Version, UpdateUrl: meta.UpdateUrl}, disableAutoRestart, p)
		if restart {
			runtime.Stop()
			return
		}
		select {
		case <-runtime.Context().Done():
			return
		case <-time.After(time.Hour):
		}
	}
}
