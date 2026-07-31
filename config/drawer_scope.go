package config

import (
	"fmt"
	"strings"
)

// DrawerPageScope declares what a drawer page is ABOUT — the subject its panes
// report on. It is metadata with small behaviors, not a routing rule: it never
// reorders pages and never switches them. What it buys is three things a page
// could not say before — a page-level sentence explaining WHY a page is empty
// and what to do about it, a visual grouping of adjacent same-subject pages in
// the page map, and the ability to dim a whole page as a unit when its subject
// is simply not there.
//
// The vocabulary is the context axis a user actually moves along: what is
// happening across grove, in this workspace, in this worktree, in the agent I
// am watching, across the agents I am running. Mixed is the honest sixth
// value — the built-in sessions dashboard really does span all of them — and it
// is the default, so an unset scope claims nothing rather than claiming wrong.
type DrawerPageScope string

const (
	// DrawerScopeGlobal: the page is about grove as a whole. Its subject is
	// always present, so it can never be dimmed for absence.
	DrawerScopeGlobal DrawerPageScope = "global"
	// DrawerScopeWorkspace: the page is about the active workspace.
	DrawerScopeWorkspace DrawerPageScope = "workspace"
	// DrawerScopeWorktree: the page is about the code checked out right here —
	// git state, changes, the repos in scope.
	DrawerScopeWorktree DrawerPageScope = "worktree"
	// DrawerScopeAgent: the page is about the ONE focused agent session. This is
	// the scope whose subject goes absent most often, and the reason page-level
	// reasons exist at all: with nothing focused, every pane on such a page is
	// empty for the same single reason.
	DrawerScopeAgent DrawerPageScope = "agent"
	// DrawerScopeAgents: the page is about several agents at once. Distinct from
	// agent because "no agent focused" does not make it empty — a multi-agent
	// view has a subject as long as the host is running.
	DrawerScopeAgents DrawerPageScope = "agents"
	// DrawerScopeMixed: the page deliberately spans scopes. The default.
	DrawerScopeMixed DrawerPageScope = "mixed"
)

// drawerPageScopes is the accepted set, in the order the axis is usually read
// (widest subject first). Used by Validate for its error message so the list a
// user is shown can never drift from the list that is enforced.
var drawerPageScopes = []DrawerPageScope{
	DrawerScopeGlobal,
	DrawerScopeWorkspace,
	DrawerScopeWorktree,
	DrawerScopeAgent,
	DrawerScopeAgents,
	DrawerScopeMixed,
}

// Validate reports whether the value is one of the accepted scopes. The empty
// value is valid and means "unset", which resolves to mixed.
func (s DrawerPageScope) Validate() error {
	if s == "" {
		return nil
	}
	for _, known := range drawerPageScopes {
		if s == known {
			return nil
		}
	}
	names := make([]string, 0, len(drawerPageScopes))
	for _, known := range drawerPageScopes {
		names = append(names, string(known))
	}
	return fmt.Errorf("drawer page scope %q: want one of %s", string(s), strings.Join(names, ", "))
}

// Resolved returns the scope a host should treat the page as having: the value
// itself when it is one of the accepted ones, and mixed for both the unset and
// the misspelled case. A bad spelling costs you the grouping, never the page —
// the same bargain [DrawerFilesConfig.View] and [DrawerPageConfig.Size] strike.
func (s DrawerPageScope) Resolved() DrawerPageScope {
	if s.Validate() != nil {
		return DrawerScopeMixed
	}
	if s == "" {
		return DrawerScopeMixed
	}
	return s
}
