package models

// TmuxSession represents a tmux session configuration
type TmuxSession struct {
	Key         string `json:"key"`
	Path        string `json:"path"`
	Repository  string `json:"repository"`
	Description string `json:"description,omitempty"`
}

// GitStatus represents git repository status
type GitStatus struct {
	Repository string `json:"repository"`
	Status     string `json:"status"`
	HasChanges bool   `json:"hasChanges"`
	IsClean    bool   `json:"isClean"`
}