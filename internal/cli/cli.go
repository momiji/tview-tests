// Package cli parses the command-line flags into a config.CmdArgs and decides
// which action to run (start the proxy, show help/version, or encrypt a
// password). It owns the usage/help/version text.
package cli

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/howeyc/gopass"

	"test/internal/config"
	"test/internal/service/secret"
)

// Meta is the application metadata used in the help text and defaults.
type Meta struct {
	Name          string
	Version       string
	Url           string
	UpdateUrl     string
	DefaultDomain string
}

// Action is what the CLI decided to do.
type Action int

const (
	ActionRun Action = iota
	ActionHelp
	ActionVersion
	ActionEncrypt
)

// Parse parses os.Args into a CmdArgs and an Action. For ActionRun it returns
// the fully resolved CmdArgs (config path defaulted, single-proxy fields
// derived). It does not exit.
func Parse(meta Meta) (config.CmdArgs, Action, error) {
	var args config.CmdArgs
	var proxy string
	var encrypt, showHelp, showVersion bool

	fs := flag.NewFlagSet(meta.Name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Print(Usage(meta)) }
	fs.StringVar(&args.Config, "c", "", "")
	fs.StringVar(&args.Config, "config", "", "")
	fs.StringVar(&args.KeyFile, "k", meta.Name+".key", "")
	fs.StringVar(&args.KeyFile, "key", meta.Name+".key", "")
	fs.StringVar(&args.Listen, "l", "", "")
	fs.StringVar(&args.Listen, "listen", "", "")
	fs.StringVar(&args.User, "u", "", "")
	fs.StringVar(&args.User, "user", "", "")
	fs.IntVar(&args.Timeout, "timeout", 3600, "")
	fs.BoolVar(&encrypt, "e", false, "")
	fs.BoolVar(&encrypt, "encrypt", false, "")
	fs.BoolVar(&args.Debug, "d", false, "")
	fs.BoolVar(&args.Debug, "debug", false, "")
	fs.BoolVar(&args.Trace, "t", false, "")
	fs.BoolVar(&args.Trace, "trace", false, "")
	fs.BoolVar(&args.Verbose, "v", false, "")
	fs.BoolVar(&args.Verbose, "verbose", false, "")
	fs.BoolVar(&showHelp, "h", false, "")
	fs.BoolVar(&showHelp, "help", false, "")
	fs.BoolVar(&showVersion, "V", false, "")
	fs.BoolVar(&showVersion, "version", false, "")
	fs.StringVar(&args.ACL, "acl", "", "")
	fs.BoolVar(&args.ConsoleUI, "ui", false, "")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return args, ActionRun, err
	}
	rest := fs.Args()

	switch {
	case showHelp:
		return args, ActionHelp, nil
	case showVersion:
		return args, ActionVersion, nil
	case encrypt:
		return args, ActionEncrypt, nil
	case len(rest) == 1 && args.Config != "":
		return args, ActionRun, fmt.Errorf("invalid arguments")
	case len(rest) > 1:
		return args, ActionRun, fmt.Errorf("invalid arguments")
	case len(rest) == 1:
		proxy = rest[0]
	}

	if proxy == "" {
		if args.Config == "" {
			if _, err := os.Stat(meta.Name + ".yaml"); err == nil {
				args.Config = meta.Name + ".yaml"
			} else if _, err := os.Stat(meta.Name + ".json"); err == nil {
				args.Config = meta.Name + ".json"
			} else {
				args.Config = meta.Name + ".yaml"
			}
		}
		args.Timeout = 0
	} else {
		args.Config = ""
		args.Verbose = true
		if args.Listen == "" {
			args.Listen = ":"
		}
		h, p := splitHostPort(args.Listen, "127.0.0.1", "8080", true)
		args.Listen = h + ":" + p
		args.BindHost = h
		args.BindPort, _ = strconv.Atoi(p)
		h, p = splitHostPort(proxy, "127.0.0.1", "8080", true)
		args.ProxyHost = h
		args.ProxyPort, _ = strconv.Atoi(p)
		if args.User != "" {
			args.Login, args.Domain = splitUsername(args.User, "")
			if args.Domain == "" {
				return args, ActionRun, fmt.Errorf("invalid value %q for flag -u: missing domain", args.User)
			}
			if !strings.Contains(args.Domain, ".") {
				args.Domain = args.Domain + meta.DefaultDomain
			}
		}
	}
	return args, ActionRun, nil
}

// EncryptPassword prompts for a password and prints its encrypted form (the
// `-e` command), using the key at args.KeyFile.
func EncryptPassword(args config.CmdArgs) error {
	fmt.Printf("Encrypt a password - key location is `%s`\n", args.KeyFile)
	fmt.Print("Password: ")
	pwdBytes, err := gopass.GetPasswdMasked() // password always exists even on error
	if err != nil {
		return err
	}
	cipher := secret.New(args.KeyFile)
	fmt.Printf("Encrypted: %s%s\n", secret.Prefix, cipher.Encrypt(string(pwdBytes)))
	return nil
}

func Version(meta Meta) string {
	return render(versionTemplate, meta)
}

func Usage(meta Meta) string {
	return fmt.Sprintf("\n%s\n%s\n", render(versionTemplate, meta), render(usageTemplate, meta))
}

func Help(meta Meta) string {
	return fmt.Sprintf("%s\n%s\n%s\n", render(versionTemplate, meta), render(usageTemplate, meta), render(helpTemplate, meta))
}

func render(text string, meta Meta) string {
	values := map[string]string{
		"AppName":          meta.Name,
		"AppUrl":           meta.Url,
		"AppDefaultDomain": meta.DefaultDomain,
		"AppVersion":       meta.Version,
	}
	var tpl bytes.Buffer
	_ = template.Must(template.New("").Parse(text)).Execute(&tpl, values)
	return tpl.String()
}

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
