package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/palantir/stacktrace"
	yaml2 "gopkg.in/yaml.v2"
)

// Load reads, validates and resolves a configuration into a ProxyConf. When
// args.Config is empty, the configuration is synthesized from a single proxy
// described by the command-line arguments; otherwise it is read from the
// file at args.Config. It performs no network access.
func Load(args CmdArgs) (*ProxyConf, error) {
	var fc FileConfig
	var err error
	if args.Config == "" {
		err = readFromArgs(&fc, args)
	} else {
		err = readFromFile(&fc, args.Config, args)
	}
	if err != nil {
		return nil, stacktrace.Propagate(err, "unable to read config")
	}
	if err = check(&fc); err != nil {
		return nil, stacktrace.Propagate(err, "invalid config")
	}
	pc, err := build(args, &fc)
	if err != nil {
		return nil, stacktrace.Propagate(err, "unable to build config")
	}
	return pc, nil
}

// readFromArgs synthesizes a configuration from a single proxy described on
// the command line: direct when no proxy port, anonymous when no login,
// kerberos otherwise.
func readFromArgs(fc *FileConfig, args CmdArgs) error {
	fc.Bind = args.BindHost
	fc.Port = args.BindPort

	fc.Proxies = map[string]*FileProxy{}
	fc.Credentials = map[string]*FileCred{}
	proxyName := "proxy"

	if args.ProxyPort == 0 {
		// consider proxy is a direct proxy
		proxyName = "direct"
	} else if args.Login == "" {
		// consider proxy is a simple anonymous proxy
		proxyType := ProxyAnonymous
		fc.Proxies[proxyName] = &FileProxy{
			Type: &proxyType,
			Host: &args.ProxyHost,
			Port: args.ProxyPort,
		}
	} else {
		// consider proxy is a kerberos proxy
		proxyType := ProxyKerberos
		proxySpn := "HTTP"
		proxyCred := "user"
		fc.Proxies[proxyName] = &FileProxy{
			Type:       &proxyType,
			Spn:        &proxySpn,
			Realm:      &args.Domain,
			Host:       &args.ProxyHost,
			Port:       args.ProxyPort,
			Credential: &proxyCred,
		}
		// add kerberos credential
		fc.Credentials[proxyCred] = &FileCred{
			Login: &args.Login,
		}
	}

	fc.Rules = make([]*FileRule, 1)
	ruleHost := "*"
	fc.Rules[0] = &FileRule{
		Host:  &ruleHost,
		Proxy: &proxyName,
	}

	if args.ACL != "" {
		fc.ACL = strings.Split(args.ACL, ",")
	}

	return nil
}

func readFromFile(fc *FileConfig, filename string, args CmdArgs) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return stacktrace.Propagate(err, "unable to read file")
	}
	if strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		err = json.Unmarshal(data, fc)
	} else {
		err = yaml2.Unmarshal(data, fc)
	}
	if err != nil {
		return stacktrace.Propagate(err, "unable to read file as yaml/json")
	}
	if args.Listen != "" {
		h, p := splitHostPort(args.Listen, "127.0.0.1", "0", true)
		fc.Bind = h
		fc.Port, _ = strconv.Atoi(p)
	}
	if args.User != "" {
		for _, cred := range fc.Credentials {
			// auto-fill missing login
			if cred.Login == nil {
				user := args.User
				cred.Login = &user
				cred.Password = nil
			}
		}
	}
	return nil
}

func splitCredentials(creds *string) []string {
	if creds != nil && *creds != "" {
		c := strings.Split(*creds, " ")
		if strings.Contains(*creds, ",") {
			c = strings.Split(*creds, ",")
		}
		return c
	}
	return nil
}

func splitFirst(proxy string) string {
	return strings.Split(proxy, ",")[0]
}

func splitComma(proxy string) []string {
	return strings.Split(proxy, ",")
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
