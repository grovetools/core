package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"

	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/paths"
)

// ConfigHook defines a hook that runs when specific config sections change.
type ConfigHook struct {
	Section string   // Config section that triggers this hook (e.g., "keys.tmux")
	Command []string // Command to execute (e.g., ["grove", "keys", "generate", "tmux"])
	Name    string   // Human-readable name for logging
}

// DefaultHooks contains the default hooks for config regeneration.
var DefaultHooks = []ConfigHook{
	{
		Section: "keys.tmux",
		Command: []string{"grove", "keys", "generate", "tmux"},
		Name:    "tmux popup bindings",
	},
	{
		Section: "nav",
		Command: []string{"nav", "key", "regenerate"},
		Name:    "nav key bindings",
	},
}

// ConfigWatcher watches the config directory for changes and triggers hooks.
type ConfigWatcher struct {
	watcher    *fsnotify.Watcher
	hooks      []ConfigHook
	debounceMs int
	mu         sync.Mutex
	// pending accumulates the files changed since the last flush, and timer
	// fires the flush once the writes go quiet. The debounce is TRAILING, not
	// leading: see handleChange.
	pending      map[string]struct{}
	timer        *time.Timer
	logger       *logrus.Entry
	onReload     func(file string) // Callback to broadcast event
	targetToLink map[string]string // Maps target file paths to their symlink names in config dir
	configDir    string            // The main config directory
}

// NewConfigWatcher creates a new ConfigWatcher that watches ~/.config/grove/.
// The debounceMs parameter controls how long to wait before processing rapid changes.
// The onReload callback is called when config changes occur (for broadcasting to clients).
// It also watches symlink target directories so changes to linked files are detected.
func NewConfigWatcher(debounceMs int, onReload func(string)) (*ConfigWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	logger := logging.NewLogger("config-watcher")
	configDir := paths.ConfigDir()

	// Watch the main config directory
	if err := watcher.Add(configDir); err != nil {
		watcher.Close()
		return nil, err
	}

	// Find symlinks and watch their target directories too
	// fsnotify doesn't follow symlinks, so we need to watch targets explicitly
	watchedDirs := make(map[string]bool)
	watchedDirs[configDir] = true
	targetToLink := make(map[string]string)

	entries, err := os.ReadDir(configDir)
	if err == nil {
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".toml") && !strings.HasSuffix(entry.Name(), ".yml") {
				continue
			}

			fullPath := filepath.Join(configDir, entry.Name())
			info, err := entry.Info()
			if err != nil {
				continue
			}

			// Check if it's a symlink
			if info.Mode()&os.ModeSymlink != 0 {
				target, err := filepath.EvalSymlinks(fullPath)
				if err != nil {
					logger.WithError(err).Warnf("Failed to resolve symlink %s", entry.Name())
					continue
				}

				// Map the target path to the symlink name
				targetToLink[target] = entry.Name()

				targetDir := filepath.Dir(target)
				if !watchedDirs[targetDir] {
					if err := watcher.Add(targetDir); err != nil {
						logger.WithError(err).Warnf("Failed to watch symlink target dir %s", targetDir)
					} else {
						watchedDirs[targetDir] = true
						logger.Debugf("Watching symlink target directory: %s", targetDir)
					}
				}
			}
		}
	}

	if debounceMs <= 0 {
		debounceMs = 100
	}

	return &ConfigWatcher{
		watcher:      watcher,
		hooks:        DefaultHooks,
		debounceMs:   debounceMs,
		pending:      make(map[string]struct{}),
		logger:       logger,
		onReload:     onReload,
		targetToLink: targetToLink,
		configDir:    configDir,
	}, nil
}

// AddHook adds a custom hook to the watcher.
func (w *ConfigWatcher) AddHook(hook ConfigHook) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hooks = append(w.hooks, hook)
}

// Start begins watching for config changes. It blocks until the context is cancelled.
func (w *ConfigWatcher) Start(ctx context.Context) {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Skip CHMOD-only events — macOS fires these frequently due to
			// metadata updates (Spotlight, backup tools, access time changes)
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}

			w.logger.Debugf("fsnotify event: %s op=%v", event.Name, event.Op)

			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				if strings.HasSuffix(event.Name, ".toml") || strings.HasSuffix(event.Name, ".yml") || strings.HasSuffix(event.Name, ".yaml") {
					// Map target file changes back to symlink names
					displayName := event.Name
					if linkName, ok := w.targetToLink[event.Name]; ok {
						displayName = filepath.Join(w.configDir, linkName)
						w.logger.Debugf("Mapped symlink target %s -> %s", event.Name, linkName)
					}
					w.handleChange(displayName)
				}
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Errorf("Watcher error: %v", err)
		case <-ctx.Done():
			w.stopDebounce()
			w.watcher.Close()
			return
		}
	}
}

// handleChange records a config file change and arms the debounce.
//
// The debounce is TRAILING and COALESCING: a change inside the quiet window
// extends the window, and every distinct file that changed during it is
// processed once when the writes stop. It used to be leading-and-dropping —
// the first write in a window was processed and every later one discarded —
// which silently lost real reloads:
//
//	`grove ecosystem materialize` writes machine.toml, clones, then appends the
//	peer subscription to sync.toml. Against local remotes those two writes land
//	~90ms apart, inside the default 100ms window, so the daemon reloaded on the
//	machine.toml it saw BEFORE the subscription existed and never saw the
//	sync.toml write at all. The machine's presence note went on reporting the
//	ecosystem as declared-missing until the next daemon restart. A dropped
//	debounce is not a delayed reload, it is a missing one.
//
// (Found by grove's identity-unified-loop E2E.)
func (w *ConfigWatcher) handleChange(file string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending[file] = struct{}{}
	delay := time.Duration(w.debounceMs) * time.Millisecond
	if w.timer == nil {
		w.timer = time.AfterFunc(delay, w.flush)
		return
	}
	w.timer.Reset(delay)
}

// flush processes every file that changed during the quiet window.
func (w *ConfigWatcher) flush() {
	w.mu.Lock()
	files := make([]string, 0, len(w.pending))
	for f := range w.pending {
		files = append(files, f)
	}
	w.pending = make(map[string]struct{})
	w.mu.Unlock()

	// Deterministic order: hooks and reload callbacks are observable, and a
	// map's iteration order would make the daemon's log non-reproducible.
	sort.Strings(files)

	for _, file := range files {
		w.logger.Infof("Config changed: %s", filepath.Base(file))

		// Run hooks if affected
		for _, hook := range w.hooks {
			if w.sectionAffected(file, hook.Section) {
				w.logger.Infof("Running config hook: %s", hook.Name)
				cmd := exec.Command(hook.Command[0], hook.Command[1:]...) //nolint:gosec // hook commands from trusted config
				if err := cmd.Run(); err != nil {
					w.logger.Errorf("Hook %s failed: %v", hook.Name, err)
				}
			}
		}

		// Trigger broadcast callback
		if w.onReload != nil {
			w.onReload(filepath.Base(file))
		}
	}
}

// stopDebounce cancels a pending flush. Called when the watcher shuts down so
// a timer cannot fire a callback against a torn-down daemon.
func (w *ConfigWatcher) stopDebounce() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
}

// sectionAffected checks if a file change affects a given config section.
func (w *ConfigWatcher) sectionAffected(file, section string) bool {
	base := filepath.Base(file)
	// Main config affects everything
	if base == "grove.toml" || base == "grove.yml" || base == "grove.yaml" {
		return true
	}

	// Check if file content contains the base section key (e.g., "keys")
	content, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	sectionRoot := strings.Split(section, ".")[0]
	return strings.Contains(string(content), sectionRoot)
}

// Close stops the watcher and releases resources.
func (w *ConfigWatcher) Close() error {
	return w.watcher.Close()
}
