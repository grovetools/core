package models

import "time"

// NoteIndexEntry represents a single note's cached metadata in the daemon's note index.
// This enables the TUI to fetch pre-parsed metadata in a single API call,
// skipping all filesystem I/O on startup.
type NoteIndexEntry struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	Title      string    `json:"title"`
	Tags       []string  `json:"tags,omitempty"`
	ID         string    `json:"id,omitempty"`
	PlanRef    string    `json:"plan_ref,omitempty"`
	Created    time.Time `json:"created,omitempty"`
	ModTime    time.Time `json:"mod_time"`
	Type       string    `json:"type"`        // "note", "plan", "artifact", "generic"
	Group      string    `json:"group"`       // relative path category: "inbox", "plans/my-plan"
	Workspace  string    `json:"workspace"`
	ContentDir string    `json:"content_dir"` // "notes", "plans", "chats"
}
