package daemon

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// reloadRecorder collects the files the watcher reported, so a test can wait
// for a specific one rather than for "any" event.
type reloadRecorder struct {
	mu    sync.Mutex
	files []string
}

func (r *reloadRecorder) record(file string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files = append(r.files, file)
}

func (r *reloadRecorder) sawWithin(d time.Duration, name string) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, f := range r.files {
			if filepath.Base(f) == name {
				r.mu.Unlock()
				return true
			}
		}
		r.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func (r *reloadRecorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.files...)
}

// startTestWatcher points the watcher at a sandboxed config dir and runs it.
func startTestWatcher(t *testing.T, home string, debounceMs int) (*reloadRecorder, string) {
	t.Helper()
	configDir := filepath.Join(home, "config", "grove")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GROVE_HOME", home)

	rec := &reloadRecorder{}
	w, err := NewConfigWatcher(debounceMs, rec.record)
	if err != nil {
		t.Fatalf("NewConfigWatcher: %v", err)
	}
	if w.configDir != configDir {
		t.Fatalf("watcher resolved config dir %s, want %s", w.configDir, configDir)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go w.Start(ctx)
	// Let the watcher goroutine reach its select before the test writes.
	time.Sleep(100 * time.Millisecond)
	return rec, configDir
}

func TestConfigWatcherReportsASingleChange(t *testing.T) {
	rec, configDir := startTestWatcher(t, t.TempDir(), 50)

	if err := os.WriteFile(filepath.Join(configDir, "sync.toml"), []byte("server = \"http://x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !rec.sawWithin(5*time.Second, "sync.toml") {
		t.Fatalf("sync.toml change was not reported; saw %v", rec.seen())
	}
}

// The regression. Two DIFFERENT config files written inside one debounce
// window must both be reported.
//
// This is `grove ecosystem materialize`: it writes machine.toml, clones, then
// appends its peer subscription to sync.toml. Against local remotes the clone
// takes milliseconds, so both writes land inside the default 100ms window. The
// old leading-edge debounce processed machine.toml and DROPPED sync.toml, so
// the daemon reloaded on a config that predated the subscription and never
// learned about it — the machine's presence note reported the ecosystem as
// declared-missing until the daemon was restarted.
func TestConfigWatcherDoesNotDropASecondFileInsideTheDebounceWindow(t *testing.T) {
	rec, configDir := startTestWatcher(t, t.TempDir(), 200)

	if err := os.WriteFile(filepath.Join(configDir, "machine.toml"), []byte("[machine]\nname = \"a\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Well inside the 200ms window — this is the write that used to vanish.
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(configDir, "sync.toml"), []byte("server = \"http://x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !rec.sawWithin(5*time.Second, "machine.toml") {
		t.Fatalf("machine.toml change was not reported; saw %v", rec.seen())
	}
	if !rec.sawWithin(5*time.Second, "sync.toml") {
		t.Fatalf("the second file written inside the debounce window was DROPPED; saw %v", rec.seen())
	}
}

// Coalescing still holds: a burst of writes to ONE file is one reload, not one
// per write. That is what the debounce is for, and the fix must not trade it
// away for correctness.
func TestConfigWatcherCoalescesAWriteBurst(t *testing.T) {
	rec, configDir := startTestWatcher(t, t.TempDir(), 150)

	path := filepath.Join(configDir, "grove.toml")
	for i := 0; i < 10; i++ {
		if err := os.WriteFile(path, []byte("# write\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !rec.sawWithin(5*time.Second, "grove.toml") {
		t.Fatalf("burst produced no reload; saw %v", rec.seen())
	}
	// Let any further flush land before counting.
	time.Sleep(400 * time.Millisecond)

	var count int
	for _, f := range rec.seen() {
		if filepath.Base(f) == "grove.toml" {
			count++
		}
	}
	if count > 2 {
		t.Fatalf("a 10-write burst produced %d reloads; the debounce is not coalescing", count)
	}
}
