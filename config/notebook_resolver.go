package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/grovetools/core/util/pathutil"
)

// THE repo→notebook resolver. Everything that needs to answer "which notebook
// does this path belong to?" goes through ResolveNotebook.
//
// It replaces two divergent implementations that answered the same question
// differently for years: `assignNotebookName`/`matchGroveNotebook` in
// core/pkg/workspace (four fallback rungs, symlink/case-normalized paths, blind
// to `enabled = false`) and `resolveNotebookContext` here in core/config
// (longest grove prefix only, raw path comparison, honoring `enabled`). Their
// disagreements were invisible: the same repo could be "grovetools" to the TUI
// and "nb" to the config loader. Both are gone — what is left of each is a call
// site: applyNotebookBinding in workspace (which contributes topology) and
// notebookWorkspaceContext here (which contributes a workspace name).
//
// HOME PACKAGE. The resolver lives in `config`, not `workspace`, because
// `workspace` already imports `config` (the reverse would be an import cycle)
// and because every input it reasons over — groves, notebook definitions,
// ecosystem cards, machine.toml — is a config concept. What `config` cannot
// compute is path TOPOLOGY: which repo owns a worktree. That stays in
// `workspace` and is handed in as NotebookQuery.OwnerPaths, so the dependency
// arrow never has to reverse.
//
// PRECEDENCE (contract §1 Q3, §3 P4), applied rung by rung; within a rung every
// candidate path is tried in order (query path first, then owner paths):
//
//  1. machine.toml override — this machine's explicit say over an ecosystem.
//  2. ecosystem card — the repo-side binding that travels with clones.
//  3. grove entry `notebook` — the machine-side binding, legacy home of both.
//  4. notebook `root_dir` containment — a path inside a notebook's own storage
//     tree resolves to THAT notebook, whatever the groves say.
//  5. `notebooks.rules.default`.

// NotebookSource names the rung that produced a binding. It exists so callers
// (and tests, and `grove doctor`) can say WHY a path resolved the way it did
// rather than just what it resolved to.
type NotebookSource string

const (
	// NotebookSourceNone means nothing bound a notebook, not even a default.
	NotebookSourceNone NotebookSource = ""
	// NotebookSourceCard is an ecosystem card's default notebook.
	NotebookSourceCard NotebookSource = "ecosystem-card"
	// NotebookSourceGrove is a grove entry's `notebook` key.
	NotebookSourceGrove NotebookSource = "grove"
	// NotebookSourceNotebookRoot is containment in a notebook's own root_dir.
	NotebookSourceNotebookRoot NotebookSource = "notebook-root"
	// NotebookSourceDefault is notebooks.rules.default.
	NotebookSourceDefault NotebookSource = "default"
)

// NotebookQuery is a repo→notebook question.
type NotebookQuery struct {
	// Path is the path being resolved.
	Path string

	// OwnerPaths are fallback paths consulted, in order, when Path itself
	// binds nothing at a given rung. They exist for worktrees: an XDG worktree
	// (~/.local/share/grove/worktrees/<eco>-<hash>/<plan>/<repo>) lives outside
	// every grove and carries no card, so its identity has to come from the
	// repo that owns it. Callers in `workspace` fill this from WorktreeOwner
	// and the node's parent/root ecosystem paths — the topology this package
	// cannot see. Empty is normal for non-worktree queries.
	OwnerPaths []string
}

// NotebookBinding is the answer. A binding with an empty Notebook means no rung
// matched and no default was configured; callers decide whether that is fatal.
type NotebookBinding struct {
	// Notebook is the resolved notebook name (a key in
	// notebooks.definitions). It is NOT validated against the definitions:
	// naming an undefined notebook is a configuration error the caller reports
	// in its own terms, exactly as both predecessors did.
	Notebook string

	// NotebookRoot is the resolved root carried by the compiled recorded view.
	// It is additive during the cutover: legacy-only bindings may still obtain
	// it from notebooks.definitions, while recorded grove/default bindings no
	// longer require that compatibility projection.
	NotebookRoot string

	// Source is the rung that decided it.
	Source NotebookSource

	// MatchedPath is the candidate path that produced the binding.
	MatchedPath string

	// GroveName and GroveRoot describe the grove containing the QUERY path (not
	// the owner paths), when one does. They are reported regardless of which
	// rung won, because callers that need a workspace name derive it from the
	// grove root. GroveRoot is the DECLARED path (absolute and expanded, but in
	// its original spelling) so it is safe to render and to join onto.
	GroveName string
	GroveRoot string

	// groveRootMatch is GroveRoot in comparison form (symlinks resolved,
	// case-folded where the filesystem is). It is unexported because it is a
	// matching artifact, not a path: on macOS it is lowercased, so rendering it
	// or deriving a workspace directory name from it would silently rename
	// things.
	groveRootMatch string

	// EcosystemRoot is the directory whose card supplied the binding, set only
	// for NotebookSourceCard.
	EcosystemRoot string
}

// ResolveNotebook answers a repo→notebook query against a config.
func ResolveNotebook(q NotebookQuery, cfg *Config) NotebookBinding {
	candidates := make([]string, 0, 1+len(q.OwnerPaths))
	seen := make(map[string]bool, 1+len(q.OwnerPaths))
	for _, p := range append([]string{q.Path}, q.OwnerPaths...) {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		candidates = append(candidates, p)
	}
	if len(candidates) == 0 {
		return NotebookBinding{}
	}

	// The grove match is computed once per candidate: three rungs consult it
	// (the machine rung to scope an override, the card rung to bound its
	// upward walk, and rung 3 itself).
	matches := make([]groveMatch, len(candidates))
	for i, c := range candidates {
		matches[i] = matchGrove(c, cfg)
	}

	binding := NotebookBinding{}
	if matches[0].root != "" {
		binding.GroveName = matches[0].name
		binding.GroveRoot = matches[0].declaredRoot
		binding.groveRootMatch = matches[0].root
	}

	// Rung 1 — ecosystem card. The walk is bounded by the containing grove so
	// resolving a path never turns into a walk to the filesystem root when a
	// grove already tells us where the ecosystem tree stops.
	for i, c := range candidates {
		if nb, ecoRoot := cardNotebook(c, matches[i].root); nb != "" {
			binding.Notebook = nb
			binding.NotebookRoot = notebookRootForName(nb, cfg)
			binding.Source = NotebookSourceCard
			binding.MatchedPath = c
			binding.EcosystemRoot = ecoRoot
			return binding
		}
	}

	// Rung 3 — grove entry. A grove that matches but declares no notebook does
	// NOT end the search: it falls through to the remaining rungs, which is how
	// a worktree under a notebook-less grove still finds its notebook.
	for i, c := range candidates {
		if matches[i].notebook != "" {
			binding.Notebook = matches[i].notebook
			binding.NotebookRoot = matches[i].notebookRoot
			binding.Source = NotebookSourceGrove
			binding.MatchedPath = c
			return binding
		}
	}

	// Rung 4 — the path lives inside a recorded notebook storage tree.
	// compileCodeRoots carries each literal routed NotebookRoot; raw legacy
	// definition containment is deliberately not a routing source.
	for _, c := range candidates {
		if nb := matchCompiledNotebookRoot(c, cfg); nb != "" {
			binding.Notebook = nb
			binding.NotebookRoot = notebookRootForName(nb, cfg)
			binding.Source = NotebookSourceNotebookRoot
			binding.MatchedPath = c
			return binding
		}
	}

	// Rung 5 — the configured default.
	if cfg != nil && cfg.Notebooks != nil && cfg.Notebooks.Rules != nil && cfg.Notebooks.Rules.Default != "" {
		binding.Notebook = cfg.Notebooks.Rules.Default
		binding.NotebookRoot = notebookRootForName(binding.Notebook, cfg)
		binding.Source = NotebookSourceDefault
		binding.MatchedPath = candidates[0]
	}
	return binding
}

// groveMatch is the grove containing a path, if any. root is the comparison
// form; declaredRoot is the same directory in its original spelling.
type groveMatch struct {
	name         string
	root         string
	declaredRoot string
	notebook     string
	notebookRoot string
}

// matchGrove returns the most specific enabled grove containing path.
//
// Two properties are inherited deliberately, one from each predecessor:
// disabled groves are skipped (config-side behavior — an operator who wrote
// `enabled = false` meant it), and paths are compared after symlink/case
// normalization (workspace-side behavior — without it a symlinked $HOME or a
// case-variant spelling silently matches nothing).
func matchGrove(path string, cfg *Config) groveMatch {
	if cfg == nil || len(cfg.Groves) == 0 || path == "" {
		return groveMatch{}
	}

	target := normalizeForMatch(path)
	if target == "" {
		return groveMatch{}
	}

	var best groveMatch
	var bestLen int
	// Sorted iteration keeps the answer deterministic when two groves are
	// configured at the same path.
	for _, name := range sortedKeys(cfg.Groves) {
		grove := cfg.Groves[name]
		if grove.Enabled != nil && !*grove.Enabled {
			continue
		}
		declared := expandPath(grove.Path)
		if abs, err := filepath.Abs(declared); err == nil {
			declared = abs
		}
		root := normalizeForMatch(declared)
		if root == "" || !pathContains(root, target) {
			continue
		}
		if len(root) > bestLen {
			bestLen = len(root)
			best = groveMatch{
				name:         name,
				root:         root,
				declaredRoot: declared,
				notebook:     grove.Notebook,
				notebookRoot: grove.NotebookRoot,
			}
		}
	}
	return best
}

// notebookRootForName resolves root metadata without requiring the projected
// legacy notebook definitions. Recorded groves carry the authoritative pair;
// Definitions remain only as an additive fallback until final cutover.
func notebookRootForName(notebook string, cfg *Config) string {
	if cfg == nil || notebook == "" {
		return ""
	}
	for _, name := range sortedKeys(cfg.Groves) {
		grove := cfg.Groves[name]
		if grove.Notebook == notebook && grove.NotebookRoot != "" {
			return grove.NotebookRoot
		}
	}
	if cfg.Notebooks != nil {
		if definition := cfg.Notebooks.Definitions[notebook]; definition != nil {
			return expandPath(definition.RootDir)
		}
	}
	return ""
}

// matchCompiledNotebookRoot consumes the resolved NotebookRoot carried by the
// compiled recorded routing view. Multiple code roots may route to the same
// notebook; deterministic iteration and longest-root selection make those
// duplicates harmless. This path is additive until the legacy Definitions
// fallback below is removed by the final cutover.
func matchCompiledNotebookRoot(path string, cfg *Config) string {
	if cfg == nil || len(cfg.Groves) == 0 || path == "" {
		return ""
	}
	target := normalizeForMatch(path)
	if target == "" {
		return ""
	}

	best := ""
	bestLen := 0
	for _, name := range sortedKeys(cfg.Groves) {
		grove := cfg.Groves[name]
		if grove.Notebook == "" || grove.NotebookRoot == "" {
			continue
		}
		root := normalizeForMatch(expandPath(grove.NotebookRoot))
		if root == "" || !pathContains(root, target) {
			continue
		}
		if len(root) > bestLen {
			bestLen = len(root)
			best = grove.Notebook
		}
	}
	return best
}

// cardNotebook walks upward from dir looking for an ecosystem manifest whose
// card binds a default notebook, and returns (notebook, ecosystem root).
//
// The walk stops after examining stopAt — the grove containing dir — because an
// ecosystem never lives above its own grove. With no grove match (a path
// outside every grove, e.g. an XDG worktree) the walk runs to the filesystem
// root; it is memoized per manifest, so the repeated walks a discovery pass
// performs cost one stat per level after the first.
func cardNotebook(dir, stopAt string) (string, string) {
	if dir == "" {
		return "", ""
	}
	current, err := filepath.Abs(dir)
	if err != nil {
		return "", ""
	}

	for {
		if nb := probeEcosystemCard(current); nb != "" {
			return nb, current
		}
		if stopAt != "" && normalizeForMatch(current) == stopAt {
			return "", ""
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", ""
		}
		current = parent
	}
}

// cardCacheEntry memoizes one manifest's parsed card binding. The stat metadata
// is what makes the memo safe: a rewritten manifest (`grove ecosystem adopt`)
// invalidates itself on the next lookup instead of surviving until the process
// restarts, which matters for the daemon.
type cardCacheEntry struct {
	modTime  time.Time
	size     int64
	notebook string
}

var cardCache sync.Map // manifest path -> cardCacheEntry

// probeEcosystemCard returns the default notebook bound by the card in dir's
// own manifest, or "" when dir has no manifest or its manifest carries no
// notebook binding. It never walks.
func probeEcosystemCard(dir string) string {
	manifest := FindEcosystemManifest(dir)
	if manifest == "" {
		return ""
	}
	info, err := os.Stat(manifest)
	if err != nil {
		return ""
	}
	if cached, ok := cardCache.Load(manifest); ok {
		entry := cached.(cardCacheEntry)
		if entry.size == info.Size() && entry.modTime.Equal(info.ModTime()) {
			return entry.notebook
		}
	}

	notebook := ""
	// A manifest that fails to parse binds nothing. Failing loudly here would
	// turn one broken grove.toml into a resolution error for every path in the
	// tree; the config loader reports that error in its own voice.
	if card, err := LoadEcosystemCard(manifest); err == nil {
		notebook = card.DefaultNotebookName()
	}
	cardCache.Store(manifest, cardCacheEntry{
		modTime:  info.ModTime(),
		size:     info.Size(),
		notebook: notebook,
	})
	return notebook
}

// ResetEcosystemCardCache drops the memoized card lookups. Tests that rewrite a
// manifest within one filesystem-timestamp tick need it; production code does
// not, because the stat check above catches real edits.
func ResetEcosystemCardCache() {
	cardCache.Range(func(key, _ any) bool {
		cardCache.Delete(key)
		return true
	})
}

// relativeWorkspaceName returns projectRoot's location inside its grove — the
// name its notebook workspace directory is keyed by — or "" when projectRoot IS
// the grove root or lies outside it.
//
// Containment is decided in comparison form (groveRootMatch) so a symlinked or
// case-variant spelling still matches, but the NAME is spelled with
// projectRoot's own characters: the comparison form is lowercased on macOS, and
// deriving a directory name from it would quietly rename `MyApp` to `myapp`.
// Component counts survive normalization (symlink resolution can only change
// the prefix, never the tail), so taking the last N components of the original
// path is exact.
func relativeWorkspaceName(groveRootMatch, projectRoot string) string {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(groveRootMatch, normalizeForMatch(absRoot))
	if err != nil || rel == "." || rel == "" || rel == string(filepath.Separator) {
		return ""
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}

	depth := len(strings.Split(rel, string(filepath.Separator)))
	parts := strings.Split(filepath.Clean(absRoot), string(filepath.Separator))
	if depth > len(parts) {
		return rel
	}
	return filepath.Join(parts[len(parts)-depth:]...)
}

// normalizeForMatch renders a path in the one spelling all containment checks
// use: absolute, symlinks resolved, case-folded on case-insensitive
// filesystems.
//
// A path that does not exist is resolved through its deepest existing ancestor
// rather than left alone. That case is the common one, not an edge: a grove
// entry can name a directory nobody has created yet, and a lookup can ask about
// a project that is about to be cloned. Resolving only whole paths would leave
// one side of the comparison with an unresolved symlink prefix (on macOS
// /var/folders/... vs /private/var/folders/...), so a child would fail to match
// the very grove that contains it.
func normalizeForMatch(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	current := abs
	for {
		if _, statErr := os.Stat(current); statErr == nil {
			resolved, nErr := pathutil.NormalizeForLookup(current)
			if nErr != nil || resolved == "" {
				break
			}
			rel, relErr := filepath.Rel(current, abs)
			if relErr != nil || rel == "." {
				return resolved
			}
			// Re-normalizing the rejoined path folds the tail's case the same
			// way the resolved prefix was folded; the prefix is already
			// symlink-free, so this cannot resolve differently.
			joined := filepath.Join(resolved, rel)
			if folded, fErr := pathutil.NormalizeForLookup(joined); fErr == nil && folded != "" {
				return folded
			}
			return joined
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	if normalized, nErr := pathutil.NormalizeForLookup(abs); nErr == nil && normalized != "" {
		return normalized
	}
	return abs
}

// pathContains reports whether target is root or lives under it. Both sides
// must already be normalized.
func pathContains(root, target string) bool {
	return target == root || strings.HasPrefix(target, root+string(filepath.Separator))
}
