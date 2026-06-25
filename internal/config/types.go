// Package config turns command-line arguments and a YAML/JSON configuration
// file into a resolved, ready-to-use proxy configuration. It models three
// layers explicitly:
//
//   - CmdArgs    — command-line arguments;
//   - FileConfig — exactly what is read from the file (stable yaml/json tags);
//   - ProxyConf  — the resolved runtime model produced by build(), with
//     effective values already computed (no tri-state pointers left).
//
// It performs no network and no runtime work: matching, PAC download/eval,
// certificate generation and host caching live in their own packages and
// run later. The pac runtime/js fields on Proxy are filled in later by the
// router (after the PAC download), not by build().
package config

import (
	"regexp"

	"test/internal/service/pac"
)

// ProxyType is the configured kind of a proxy entry.
type ProxyType string

const (
	ProxyKerberos  ProxyType = "kerberos"
	ProxySocks     ProxyType = "socks"
	ProxyAnonymous ProxyType = "anonymous"
	ProxyDirect    ProxyType = "direct"
	ProxyBasic     ProxyType = "basic"
	ProxyNone      ProxyType = "none"
	ProxyPac       ProxyType = "pac"
)

// CredentialKerberos is the reserved credential name selecting the native
// OS Kerberos implementation.
const CredentialKerberos = "kerberos"

// Prefix marker for encrypted passwords is owned by the secret package
// (secret.Prefix); build() strips it before decrypting.

// DefaultConnectTimeout bounds dialing and request-header reads, in seconds.
const DefaultConnectTimeout = 10

func (pt ProxyType) Name() string {
	return string(pt)
}

func (pt ProxyType) Value() int {
	switch pt {
	case ProxyKerberos:
		return 0
	case ProxySocks:
		return 1
	case ProxyAnonymous:
		return 2
	case ProxyDirect:
		return 3
	case ProxyBasic:
		return 4
	case ProxyNone:
		return 5
	case ProxyPac:
		return 6
	}
	return -1
}

// CmdArgs carries the command-line arguments relevant to configuration. It
// replaces the global Options. Flags are populated by the CLI layer; the
// debug/trace/verbose booleans are "force on" switches (highest priority in
// the cascade, see build()).
type CmdArgs struct {
	Config    string // config file path; empty means "synthesize from a single proxy"
	KeyFile   string // password encryption key location
	Listen    string // overrides bind host:port
	User      string // auto-fills missing credential logins
	ACL       string // comma-separated list of allowed IPs/CIDRs
	Debug     bool
	Trace     bool
	Verbose   bool
	ConsoleUI bool

	// Derived from the single-proxy positional argument when Config is empty.
	BindHost  string
	BindPort  int
	ProxyHost string
	ProxyPort int
	Login     string
	Domain    string
}

// FileConfig is the parsed file: only what is read from yaml/json. Tags are
// kept identical to the historical format so existing files parse unchanged.
// The tri-state switches (Verbose/Debug/Trace/Mitm) are *bool: nil means
// "inherit", true/false means "explicit".
type FileConfig struct {
	Bind        string
	Port        int
	SocksPort   int `yaml:"socksPort"`
	Verbose     *bool
	Debug       *bool
	Trace       *bool
	Mitm        *bool
	Proxies     map[string]*FileProxy
	Credentials map[string]*FileCred
	Domains     map[string]*string
	Rules       []*FileRule
	SocksRules  []*FileRule `yaml:"socksRules"`
	Krb5        string
	ConnectTimeout int `yaml:"connectTimeout"`
	Check          *bool
	Update         bool
	Restart        bool
	UseEnvProxy    bool
	Experimental   string   // space/comma separated list of features
	ACL            []string `yaml:"acl"`
	ConsoleUI      bool     `yaml:"ui"`
}

type FileCred struct {
	Login    *string
	Password *string
}

type FileProxy struct {
	Type        *ProxyType
	Host        *string
	Port        int
	Verbose     *bool
	Debug       *bool
	Trace       *bool
	Mitm        *bool
	Ssl         bool
	Spn         *string
	Realm       *string
	Credential  *string
	Credentials *string
	Pac         *string
	PacOrder    int `yaml:"pacOrder"`
	Url         *string
}

type FileRule struct {
	Host    *string
	Proxy   *string
	Dns     *string
	Verbose *bool
	Debug   *bool
	Trace   *bool
	Mitm    *bool
}

func (r *FileRule) firstProxy() string {
	return splitFirst(*r.Proxy)
}

func (r *FileRule) allProxiesName() []string {
	return splitComma(*r.Proxy)
}

// ProxyConf is the resolved runtime model. Effective booleans are plain
// bool (cascade already applied); credentials are linked, built-ins added.
type ProxyConf struct {
	Bind           string
	Port           int
	SocksPort      int
	ConnectTimeout int
	Verbose        bool
	Debug          bool
	Trace          bool
	Proxies        map[string]*Proxy
	Credentials    map[string]*Cred
	Domains        map[string]*string
	Rules          []*Rule
	SocksRules     []*Rule
	Krb5           string
	Check          *bool
	Update         bool
	Restart        bool
	UseEnvProxy    bool
	Experimental   string
	ACL            []string
	ConsoleUI      bool
	// derived
	PacProxy               string   // "PROXY bind:port" served as the default
	PacProxies             []*Proxy // pac proxies, ordered by PacOrder
	ExperimentalHostsCache bool     // coarse per-host match cache, disables fine url lookup
}

type Cred struct {
	Name      string
	Login     *string
	Password  *string
	IsNull    bool
	IsPerUser bool
	IsUsed    bool // set if not null, not per-user and used by a rule => proxy
	IsNative  bool // native kerberos implementation
}

type Proxy struct {
	Name        string
	Type        *ProxyType
	TypeValue   int
	Host        *string
	Port        int
	Ssl         bool
	Spn         *string
	Realm       *string
	Credential  *string
	Credentials *string
	Cred        *Cred // not nil for kerberos/basic, and for authenticated socks
	Pac         *string
	PacOrder    int
	Url         *string
	IsUsed      bool
	// effective switches (cascade applied)
	Verbose bool
	Debug   bool
	Trace   bool
	Mitm    bool
	// routing-derived (PacRegex/PacProxy by build, PacJs/PacRuntime by router)
	PacRegex   *Regex
	PacProxy   *string
	PacJs      *string
	PacRuntime *pac.PacExecutor
}

type Rule struct {
	Host  *string
	Proxy *string
	Dns   *string
	// effective switches (cascade applied)
	Verbose bool
	Debug   bool
	Trace   bool
	Mitm    bool
	// compiled host matcher
	Regex *Regex
}

// Regex is a compiled host matcher: a glob (translated to a regexp) or a raw
// `re:` regexp, optionally negated with a leading `!`.
type Regex struct {
	Pattern *regexp.Regexp
	Regex   string
	Exclude bool
}

func (r *Rule) FirstProxy() string {
	return splitFirst(*r.Proxy)
}

func (r *Rule) AllProxiesName() []string {
	return splitComma(*r.Proxy)
}
