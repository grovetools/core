package git

import (
	"fmt"
)

// BranchChangeHandler handles branch changes
type BranchChangeHandler struct{}

// NewBranchChangeHandler creates a new branch change handler
func NewBranchChangeHandler() *BranchChangeHandler {
	return &BranchChangeHandler{}
}

// HandleBranchChange responds to a branch change
func (h *BranchChangeHandler) HandleBranchChange(workDir, fromBranch, toBranch string) error {
	// For now, just print the branch change
	// The actual environment management should be handled by the CLI command
	// which can import both git and compose packages
	fmt.Printf("Grove: Detected branch change from '%s' to '%s'\n", fromBranch, toBranch)

	// TODO: In the future, this could emit an event or signal
	// that the CLI layer can respond to

	return nil
}