package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// OpenBrowser opens the specified URL in the default browser
func OpenBrowser(url string) error {
	return OpenPath(nil, url)
}

// OpenPath hands a path or URL to whatever opens it outside the terminal.
//
// openCommand is the user's configured opener argv ([tui] open_command in
// grove.toml); the target is appended as its final argument, so
// []string{"open", "-a", "Firefox"} runs `open -a Firefox <target>`. An empty
// or all-blank openCommand falls back to the platform opener, which is what
// OpenBrowser has always done.
func OpenPath(openCommand []string, target string) error {
	argv, err := openArgv(openCommand, target)
	if err != nil {
		return err
	}
	return exec.Command(argv[0], argv[1:]...).Start()
}

// openArgv resolves the full argv to run, target included.
func openArgv(openCommand []string, target string) ([]string, error) {
	argv := make([]string, 0, len(openCommand)+1)
	for _, arg := range openCommand {
		if strings.TrimSpace(arg) == "" {
			continue // a blank entry is a config typo, not an argument
		}
		argv = append(argv, arg)
	}
	if len(argv) == 0 {
		platform, err := platformOpenArgv()
		if err != nil {
			return nil, err
		}
		argv = platform
	}
	return append(argv, target), nil
}

// platformOpenArgv is the OS's own opener, as an argv the target appends to.
func platformOpenArgv() ([]string, error) {
	switch runtime.GOOS {
	case "darwin":
		return []string{"open"}, nil
	case "linux":
		return []string{"xdg-open"}, nil
	case "windows":
		return []string{"cmd", "/c", "start"}, nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
