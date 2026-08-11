package models

// NotespaceAdoption is an explicit, post-adopt signal from a Grove verb. Root
// is the adopted code checkout (not a notebook path); Subject has already been
// canonicalized by the caller. The daemon validates the checkout against a
// recorded scan root before it is allowed to materialize any notes-plane state.
type NotespaceAdoption struct {
	Root        string `json:"root"`
	Subject     string `json:"subject"`
	Kind        string `json:"kind"`
	DisplayName string `json:"display_name"`
}

// NotespaceAdoptionResult describes the notespace materialized for one trusted
// adoption signal. Minted is false when an existing valid stamp won the
// load-first race.
type NotespaceAdoptionResult struct {
	NotespaceID   string `json:"notespace_id"`
	NotespaceRoot string `json:"notespace_root"`
	Minted        bool   `json:"minted"`
}
