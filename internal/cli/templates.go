package cli

// User-facing help text. The config-format section was trimmed of the dropped
// knobs (idleTimeout/closeTimeout, connection-pools) so it does not advertise
// features that no longer exist.

const versionTemplate = "{{.AppName}} {{.AppVersion}} - {{.AppUrl}}"

const usageTemplate = `
{{.AppName}} is a Kerberos authenticating HTTP/1.1 proxy, that forwards requests to any upstream proxies and servers.
It exposes an anonymous proxy, automatically injecting required credentials when forwarding requests.
It also provides a javascript proxy.pac to be used in browser or system proxy, at 'http://HOST:PORT/proxy.pac'.

Usage: {{.AppName}} [-dtv] [-u <user@domain>] [-l <[ip:]port>] [-c <config>] [-k <key>]
       {{.AppName}} [-dtv] [-u <user@domain>] [-l <[ip:]port>] [--timeout <timeout>] [--acl <ips>] <proxy:port>
       {{.AppName}} -e [-k <key>]

Use the first form to start the proxy with a configuration file, and the second form to start the proxy with a single proxy.
In second form, the upstream proxy is of type 'kerberos' if a user is provided, and 'anonymous' otherwise, unless port number is 0 and in that case it is 'direct'.
The third form is used to encrypt a password, using the encryption key provided by '-k' option.

Example:
       {{.AppName}} -u user_login@eur -l 8888 proxy:8080

Options:
      -c, --config=<config>      config file, in yaml format (defaults to '{{.AppName}}.yaml' then '{{.AppName}}.json')
      -k, --key=<key>            encryption key location (defaults to '{{.AppName}}.key')
      -l, --listen=<[ip:]port>   listen to this ip port (ip defaults to 127.0.0.1, port defaults to 8080)
      -u, --user=<user@domain>   user for authentication, like login@domain or domain\login
                                 /!\ domain is case-sensitive in Kerberos, however it is uppercased as all internet usage seems to be uppercase
                                 domain is automatically expanded to {{.AppDefaultDomain}} when set from command line
                                 can also replace user in configuration file, when there is only one user defined
          --acl=<ips>            list of comma-separated IPs or CIDRs, who is allowed to connect
          --timeout=<timeout>    automatically stop {{.AppName}} after specified seconds, when run without config file, defaults to 3600s = 1h (set to 0 to disable)
          --ui                   enable experimental console UI
      -e, --encrypt              encrypt a password, encryption key location is {{.AppName}}.key
      -d, --debug                run in debug mode, displaying all headers
      -t, --trace                run in trace mode, displaying everything
      -v, --verbose              run in verbose mode, displaying all requests (automatically set if run without config file)
      -h, --help                 show full help with config file format
      -V, --version              show version

Note 1: remote HTTPS proxies has not been tested, as none was available for testing.
Note 2: failover proxies can be configured for a single rule "proxy: proxy1,proxy2,...", but only works for non-pac proxies, and assumes all proxies are "almost" of the same type.
Note 3: failover hosts can be configured for a single proxy "host: host1,host2,...", but only works for non-pac proxies.
`

const helpTemplate = `
CONFIG FILE
===========
A config file can be provided as json or yaml format.
Content should be similar to this:

# listen to this ip, use 0.0.0.0 to listen on all ips
bind: 127.0.0.1
# listen to this port to serve HTTP requests
port: 7777
# listen to this port to serve SOCKS requests
socksPort: 7778
# set verbose to see all requests
verbose: true
# set debug to view all requests and responses headers
debug: true
# set debug to view everything
trace: false
# timeout for connecting
connectTimeout: 10
# check for updates, defaults to true
check: true
# automatically update, defaults to false
update: false
# exit after update, defaults to false, use only if a restart mechanism is implemented outside
restart: false
# use proxy environment variables for downloading updates and pac files, defaults to false
useEnvProxy: false
# experimental features, defaults to none
experimental: hosts-cache
# experimental console ui
ui: false

# list of proxies
proxies:
# sample of a PAC proxy. 'credentials' is used to list the users we need to get login/password on startup
  pac-mkt:
    type: pac
    url: http://broproxycfg.int.world.company/ProxyPac/proxy.pac
    credentials: user
# sample of kerberos proxy. 'pac' is used to get the kerberos realm in PAC proxies at runtime
  mkt:
    type: kerberos
    spn: HTTP
    realm: EUR.MSD.WORLD.COMPANY
    host: proxy-mkt.int.world.company
    port: 8080
    credential: user
    pac: proxy-mkt*
# catch-all proxy with kerberos authentication
  any:
    type: kerberos
    spn: HTTP
    realm: EUR.MSD.WORLD.COMPANY
    host: "*"      # automatically use host:port given by PAC file
    credential: user
    pac: proxy-*   # catch all proxy-* from PAC files
    pacOrder: 100  # use this in last resort, default pacOrder is 0
# sample of anonymous (no authentication) proxy. 'ssl' for HTTPS proxy
  net:
    type: anonymous
    host: 127.0.0.1
    port: 3128
    ssl: false
# sample of socks proxy
  nets:
    type: socks
    host: localhost
    port: 1080
# sample of basic (base64 encoding) proxy. 'credential' is the user to get login/password on startup
  basic:
    type: basic
    host: proxy-mkt.int.world.company
    port: 8080
    credential: user

# list of credentials
credentials:
# sample of credential. if no 'password', it will be asked on startup. the same for login
# password can be provided as clear text, or encrypted using '-e' option
  user:
    login: a443939
    password: encrypted:SECRET_KEY

# list of rules to determine which proxy to use for HTTP proxy
rules:
# sample: direct connection for this host
  - host: "test-proxy-pac1"
    proxy: direct
    dns: 127.0.0.1
# sample: alter ip and or port resolution for this host. syntax is [IP]:[PORT]. no port means use the same port as source
  - host: "test-proxy-pac2"
    dns: 127.0.0.1:7777
# sample: multiple hosts, separated by '|'. add '!' at the beginning to inverse rule. verbosity can also be set at rule level
  - host: 192.168.2.6|osmose-homo*
    proxy: net
# sample: regex can be used for host, add 're:' at the beginning. add '!re:' to inverse rule
  - host: "re:^github\.com$|^gitlab.com$"
    proxy: mkt
    verbose: true
# sample: use mitm to have man-in-the-middle hijacked connections, CA is written in {{.AppName}}.ca.crt
  - host: "update.microsoft.com"
    proxy: mkt
    mitm: true
# sample: proxy 'none' goes nowhere, result is always 400 bad request
  - host: "microsoft.com"
    proxy: none
# sample: use '*' host as a catch all
  - host: "*"
    proxy: pac-mkt
    verbose: true

# list of rules to determine which proxy to use for SOCKS proxy
socksRules:
  - host: "*"
    proxy: net

# list some domain aliases, allowing to use 'EUR' instead of 'EUR.MSD.WORLD.COMPANY'
domains:
  EUR: EUR.MSD.WORLD.COMPANY

# list of IPs who is allowed to connect. If empty - everybody is allowed
acl:
  - 127.0.0.1
`
