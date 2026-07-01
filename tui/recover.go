package tui

import "fmt"

// RecoverView is deferred from panel/page View() methods to keep a panicking
// embedded TUI from tearing down the whole frame. Instead of silently
// returning an empty (blank) view, it writes a visible error message into
// the named return value so the user can see that the panel crashed.
func RecoverView(view *string) {
	if r := recover(); r != nil {
		*view = fmt.Sprintf("panel crashed: %v", r)
	}
}
