package config

import (
	"context"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// reloadTestTimeout is how often (seconds) the watcher re-checks the config,
// independently of file-change events, so a periodic reload callback can
// honor mod-time changes and any "fast reload" request.
const reloadTestTimeout = 10

// ReloadForceTimeout is the interval (seconds) after which a reload callback
// should reload even when the file is unchanged. Exposed so the callback can
// implement the same policy as the watcher's cadence.
const ReloadForceTimeout = 60 * 60

// Watch calls reload whenever the file at path changes (debounced) and
// periodically every reloadTestTimeout seconds, until ctx is cancelled. It
// replaces the old ManualResetEvent-based watch loops with a context-driven
// one; the actual reload policy (mod-time gating, hot-reload feasibility,
// publishing the new config) lives in the reload callback.
func Watch(ctx context.Context, path string, reload func()) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer func() { _ = watcher.Close() }()

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	_ = watcher.Add(dir)

	// debounce file-change bursts into a single reload
	debounce := time.NewTimer(time.Hour)
	debounce.Stop()
	defer debounce.Stop()

	ticker := time.NewTicker(reloadTestTimeout * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reload()
		case <-debounce.C:
			reload()
		case e, ok := <-watcher.Events:
			if !ok {
				continue
			}
			if filepath.Base(e.Name) == base && (e.Has(fsnotify.Create) || e.Has(fsnotify.Write)) {
				debounce.Reset(100 * time.Millisecond)
			}
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
		}
	}
}
