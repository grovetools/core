package config

import (
	"os"
	"path/filepath"

	"github.com/grovetools/core/pkg/coderoot"
)

// Recorded code-root reconciliation states.
const (
	CodeRootPresent         = "present"
	CodeRootDeclaredMissing = "declared-missing"
	CodeRootUnmanifested    = "unmanifested"
)

// CodeRootState is a specific recorded code root reconciled against disk.
// Scan roots are intentionally inert and therefore absent from this result.
type CodeRootState struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Notebook string   `json:"notebook,omitempty"`
	State    string   `json:"state"`
	Manifest string   `json:"manifest,omitempty"`
	Enabled  bool     `json:"enabled"`
	Repos    []string `json:"repos,omitempty"`
	Exclude  []string `json:"exclude,omitempty"`
}

func (s CodeRootState) Missing() bool {
	return s.State == CodeRootDeclaredMissing || s.State == CodeRootUnmanifested
}

// ReconcileCodeRoots reconciles materializable candidates in a recorded table.
// Specific roots (scan=false) are candidates; scan roots describe discovery
// locations and are never actionable materialization intent.
func ReconcileCodeRoots(table coderoot.Table) []CodeRootState {
	out := make([]CodeRootState, 0, len(table.Roots))
	for _, name := range table.SortedRootNames() {
		r := table.Roots[name]
		if r.Scan {
			continue
		}
		state := CodeRootState{
			Name: name, Path: expandPath(r.Path), Notebook: table.RootNotebook(name),
			Enabled: r.Enabled == nil || *r.Enabled,
			Repos:   append([]string(nil), r.Repos...), Exclude: append([]string(nil), r.Exclude...),
		}
		if abs, err := filepath.Abs(state.Path); err == nil {
			state.Path = abs
		}
		info, err := os.Stat(state.Path)
		switch {
		case err != nil || !info.IsDir():
			state.State = CodeRootDeclaredMissing
		default:
			if manifest := FindEcosystemManifest(state.Path); manifest != "" {
				state.State, state.Manifest = CodeRootPresent, manifest
			} else {
				state.State = CodeRootUnmanifested
			}
		}
		out = append(out, state)
	}
	return out
}
