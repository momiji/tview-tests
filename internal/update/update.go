// Package update implements the optional self-update: it checks the releases
// API for a newer version and, when enabled, downloads and installs the new
// binary in place. It signals (rather than performs) the restart.
package update

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"

	"test/internal/config"
	"test/internal/service/printer"
)

// Meta is the application metadata the updater needs: the binary name (used
// to pick the right release asset), the current version, and the releases API
// URL.
type Meta struct {
	Name      string
	Version   string
	UpdateUrl string
}

// Update checks for a newer release. When one is available and conf.Update is
// set, it downloads and installs it in place. It returns true when the caller
// should restart (the old behavior exited with code 200); disableAutoRestart
// suppresses that (e.g. after an interactive login).
func Update(conf *config.ProxyConf, meta Meta, disableAutoRestart bool, p *printer.Printer) bool {
	// check for updates ?
	if conf.Check != nil && *conf.Check == false {
		return false
	}
	// get update name
	list := map[string]string{
		"windows/amd64": meta.Name + ".exe",
		"linux/amd64":   meta.Name,
		"darwin/amd64":  meta.Name + ".macos",
	}
	nameOsArch := runtime.GOOS + "/" + runtime.GOARCH
	updateName, ok := list[nameOsArch]
	if !ok {
		return false
	}
	// download releases list
	url := meta.UpdateUrl
	if url == "" {
		return false
	}
	p.Infof("[-] Checking for updates: %s", url)
	httpClient := newHttpClient(conf)
	get, err := httpClient.Get(url)
	if err != nil {
		p.Errorf("[-] Update failed: %v", err)
		return false
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(get.Body)
	jsb, err := io.ReadAll(get.Body)
	if err != nil {
		p.Errorf("[-] Update failed: %v", err)
		return false
	}
	js := map[string]any{}
	err = json.Unmarshal(jsb, &js)
	if err != nil {
		p.Errorf("[-] Update failed: %v", err)
		return false
	}
	// check for new version
	ver := jsString(js, "name")
	if ver == meta.Version || ver == "v"+meta.Version {
		p.Infof("[-] No update available")
		return false
	}
	// find download url
	assetUrl := ""
	assets := jsSlice(js, "assets")
	for _, a := range assets {
		asset := jsMap(a)
		if asset != nil {
			name := jsString(asset, "name")
			if name != updateName {
				continue
			}
			assetUrl = jsString(asset, "browser_download_url")
			break
		}
	}
	if assetUrl == "" {
		p.Infof("[-] No download url available")
		return false
	}
	p.Infof("[-] New version %s found", ver)
	// automatically update ?
	if !conf.Update {
		p.Infof("[-] Skipping update (update=false)")
		return false
	}
	// download release
	p.Infof("[-] Downloading update: %s", assetUrl)
	exe, err := os.Executable()
	if err != nil {
		p.Errorf("[-] Download failed: %v", err)
		return false
	}
	stat, err := os.Stat(exe)
	if err != nil {
		p.Errorf("[-] Download failed: %v", err)
		return false
	}
	_ = os.Remove(exe + ".new")
	file, err := os.Create(exe + ".new")
	if err != nil {
		p.Errorf("[-] Download failed: %v", err)
		return false
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)
	writer := bufio.NewWriter(file)
	defer func(name string) {
		_ = os.Remove(name)
	}(file.Name())
	get, err = httpClient.Get(assetUrl)
	if err != nil {
		p.Errorf("[-] Download failed: %v", err)
		return false
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(get.Body)
	_, err = io.Copy(writer, get.Body)
	if err != nil {
		p.Errorf("[-] Download failed: %v", err)
		return false
	}
	err = writer.Flush()
	if err != nil {
		p.Errorf("[-] Download failed: %v", err)
		return false
	}
	err = file.Close()
	if err != nil {
		p.Errorf("[-] Download failed: %v", err)
		return false
	}
	// replace executable
	p.Infof("[-] Installing update: %s", exe)
	err = os.Chmod(file.Name(), stat.Mode())
	if err != nil {
		p.Errorf("[-] Install failed: %v", err)
		return false
	}
	_ = os.Remove(exe + ".old")
	err = os.Rename(exe, exe+".old")
	if err != nil {
		p.Errorf("[-] Install failed: %v", err)
		return false
	}
	err = os.Rename(exe+".new", exe)
	if err != nil {
		p.Errorf("[-] Install failed: %v", err)
		return false
	}
	// restart ?
	if !conf.Restart {
		p.Infof("[-] Skipping restart (restart=false)")
		return false
	}
	if disableAutoRestart {
		p.Infof("[-] Skipping restart (interactive login/password)")
		return false
	}
	p.Infof("[-] Exiting on update (restart=true)")
	return true
}

func newHttpClient(conf *config.ProxyConf) *http.Client {
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	if !conf.UseEnvProxy {
		httpTransport.Proxy = nil
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: httpTransport}
}

func jsString(v map[string]any, s string) string {
	if val, ok := v[s]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func jsSlice(v map[string]any, s string) []any {
	if val, ok := v[s]; ok {
		if sl, ok := val.([]any); ok {
			return sl
		}
	}
	return nil
}

func jsMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}
