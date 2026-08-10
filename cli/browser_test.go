package cli

import (
	"runtime"
	"strings"
	"testing"
)

// A configured opener is run verbatim with the target appended, so
// `open -a Firefox` reaches the browser the user actually named.
func TestOpenArgvUsesConfiguredCommand(t *testing.T) {
	argv, err := openArgv([]string{"open", "-a", "Firefox"}, "/tmp/report.html")
	if err != nil {
		t.Fatalf("openArgv: %v", err)
	}
	if got, want := strings.Join(argv, " "), "open -a Firefox /tmp/report.html"; got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
}

// Blank entries are config typos, not arguments — an all-blank command is the
// same as none at all.
func TestOpenArgvSkipsBlankEntriesAndFallsBack(t *testing.T) {
	argv, err := openArgv([]string{"open", "  "}, "/tmp/x.pdf")
	if err != nil {
		t.Fatalf("openArgv: %v", err)
	}
	if got, want := strings.Join(argv, " "), "open /tmp/x.pdf"; got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}

	argv, err = openArgv([]string{"", " "}, "/tmp/x.pdf")
	if err != nil {
		if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
			return // unsupported platform: no opener to fall back to
		}
		t.Fatalf("openArgv: %v", err)
	}
	platform, err := platformOpenArgv()
	if err != nil {
		t.Fatalf("platformOpenArgv: %v", err)
	}
	if argv[0] != platform[0] {
		t.Errorf("argv = %v, want the platform opener %v", argv, platform)
	}
	if argv[len(argv)-1] != "/tmp/x.pdf" {
		t.Errorf("argv = %v, want the target last", argv)
	}
}

// No configuration keeps the historical behavior: the platform opener with the
// URL as its final argument.
func TestOpenArgvDefaultsToPlatformOpener(t *testing.T) {
	argv, err := openArgv(nil, "https://example.com")
	if err != nil {
		t.Skipf("no platform opener on %s", runtime.GOOS)
	}
	if argv[len(argv)-1] != "https://example.com" {
		t.Errorf("argv = %v, want the URL last", argv)
	}
	switch runtime.GOOS {
	case "darwin":
		if argv[0] != "open" {
			t.Errorf("argv[0] = %q, want open", argv[0])
		}
	case "linux":
		if argv[0] != "xdg-open" {
			t.Errorf("argv[0] = %q, want xdg-open", argv[0])
		}
	}
}
