package models

// NoteEventType represents the type of note mutation.
type NoteEventType string

const (
	NoteEventCreated  NoteEventType = "created"
	NoteEventUpdated  NoteEventType = "updated"
	NoteEventDeleted  NoteEventType = "deleted"
	NoteEventMoved    NoteEventType = "moved"
	NoteEventArchived NoteEventType = "archived"
	NoteEventRenamed  NoteEventType = "renamed"
)

// NoteEvent represents a note mutation event sent from nb to the daemon.
type NoteEvent struct {
	Event             NoteEventType   `json:"event"`
	NotespaceID       string          `json:"notespace_id"`
	NotespaceName     string          `json:"notespace_name,omitempty"`
	NoteType          string          `json:"note_type"`
	Path              string          `json:"path"`
	PrevNotespaceID   string          `json:"prev_notespace_id,omitempty"`
	PrevNotespaceName string          `json:"prev_notespace_name,omitempty"`
	PrevNoteType      string          `json:"prev_note_type,omitempty"`
	PrevPath          string          `json:"prev_path,omitempty"`
	IndexEntry        *NoteIndexEntry `json:"index_entry,omitempty"` // Pre-parsed metadata for index upsert
}
