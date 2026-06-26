// Command kpx is the proxy entry point. It defines the application metadata
// (overridable at build time via -ldflags) and delegates to the app package.
package main

import (
	"os"

	"test/internal/app"
	"test/internal/cli"
)

// Application metadata. AppVersion is typically set at build time with
// -ldflags "-X main.AppVersion=...".
var (
	AppName          = "kpx"
	AppVersion       = "dev"
	AppUrl           = "https://github.com/momiji/kpx"
	AppUpdateUrl     = "https://api.github.com/repos/momiji/kpx/releases/latest"
	AppDefaultDomain = ".EXAMPLE.COM"
)

func main() {
	os.Exit(app.Main(cli.Meta{
		Name:          AppName,
		Version:       AppVersion,
		Url:           AppUrl,
		UpdateUrl:     AppUpdateUrl,
		DefaultDomain: AppDefaultDomain,
	}))
}
