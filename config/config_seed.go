package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/grovetools/core/pkg/coderoot"
)

// The shared config-seed writer.
//
// A freshly provisioned machine needs a small, deterministic set of grove
// config files before its daemon can do anything: the machine's intent
// (machine.toml), the host-level topology it cannot infer (grove.toml —
// notebook roots, daemon transport), and its sync subscriptions (sync.toml).
//
// Until this existed, `grove satellite up` wrote those files by shipping
// hand-written heredocs through the bootstrap script's SSH session
// (satellite-bootstrap.sh step 5). That had three problems worth naming,
// because they are the reason this package renders them instead:
//
//  1. The sync.toml was rendered by `printf` in bash, bypassing the role-aware
//     editor's single choke point (RenderSyncWorkspaces) — the one place that
//     enforces "a push-only role may never carry pull = true".
//  2. The grove.toml declared `[groves.*]` directly, so a satellite could not
//     express machine intent the way every other machine now does
//     (machine.toml, compiled into cfg.Groves at load).
//  3. Nothing validated the generated TOML before it landed on the target. A
//     typo surfaced as a broken daemon on a remote host.
//
// ConfigSeed renders the complete (up to five-file) seed in Go, through the same validators
// the loaders use, and hands the caller either files to write locally
// (ApplyConfigSeed) or a line-oriented bundle to ship to a remote host
// (Bundle). Nothing here knows about SSH, satellites, or transports.

// Seed file names. The set is closed on purpose: a seed may only produce
// these five files, and both the Go writer and the remote unpacker validate
// against the same list.
const (
	SeedFileGroveTOML     = "grove.toml"
	SeedFileMachineTOML   = "machine.toml"
	SeedFileRootsTOML     = coderoot.RootsFileName
	SeedFileNotebooksTOML = coderoot.NotebooksFileName
	SeedFileSyncTOML      = "sync.toml"
)

// BundleVersion is the first line of a rendered bundle. The remote unpacker
// checks it, so a newer CLI talking to an older unpacker fails loudly.
const BundleVersion = "#!grove-config-seed v1"

// BundleEnd is the sentinel that terminates a bundle on a stream shared with
// other content (the satellite bootstrap pipes the bundle and the remote
// script down one stdin).
const BundleEnd = "#!grove-config-seed-end"

// ConfigSeed is the config a freshly provisioned machine starts life with.
// Every field is optional: an empty seed renders no files.
type ConfigSeed struct {
	// Provenance is a one-line explanation of who generated this seed. It is
	// written as a comment at the top of every file, so someone reading the
	// config on the target host learns where it came from.
	Provenance string

	// MachineName is [machine] name in machine.toml.
	MachineName string
	// Ecosystems are [machine.ecosystems.<name>] entries.
	Ecosystems map[string]MachineEcosystem
	// Roots are [machine.roots.<name>] entries.
	Roots map[string]MachineRoot

	// Notebooks are the temporary legacy [notebooks.definitions.<name>]
	// values in grove.toml.
	Notebooks map[string]string

	// CodeRoots and RecordedNotebooks are the authoritative recorded routing
	// pair. RecordedDefaultNotebook names the default definition.
	CodeRoots               map[string]coderoot.Root
	RecordedNotebooks       map[string]coderoot.Notebook
	RecordedDefaultNotebook string
	// DaemonSSH writes [daemon.ssh] enabled = true.
	DaemonSSH bool
	// LegacyGroves additionally compiles Ecosystems into `[groves.<name>]`
	// entries in grove.toml.
	//
	// This exists for one concrete reason: a target host may be running a
	// grove built from a commit that predates machine.toml support, and would
	// then discover nothing at all. Explicit `[groves.*]` entries win over
	// compiled ones (compileMachineGroves fills only ABSENT keys), so writing
	// both is safe in either direction — the machine.toml is the intent, the
	// mirror is the migration-window safety net. Drop it once every target
	// runs a machine.toml-aware grove.
	LegacyGroves bool

	// Sync, when non-nil, renders sync.toml.
	Sync *SyncSeed

	// Dirs are directories the target must have, relative to its home
	// directory (e.g. notebook workspace skeletons). They are part of the seed
	// because they are the same "make this machine's config real" step, and a
	// sync subscription to a workspace whose root does not exist resolves to
	// nothing.
	Dirs []string
}

// SyncSeed is the sync.toml half of a seed.
type SyncSeed struct {
	Server       string
	TokenCommand string
	Token        string
	Workspaces   []SyncWorkspace
}

// SeedFile is one rendered config file: a basename inside the target's grove
// config directory, its content, and the mode it must land with.
type SeedFile struct {
	Name    string
	Content string
	Mode    fs.FileMode
}

// Files renders the seed. The result is deterministic (sorted keys) and every
// file has been parsed back through its real loader before being returned —
// a seed that would not load is an error here, not a broken remote daemon.
func (s ConfigSeed) Files() ([]SeedFile, error) {
	var files []SeedFile

	if grove, err := s.renderGroveTOML(); err != nil {
		return nil, err
	} else if grove != "" {
		files = append(files, SeedFile{Name: SeedFileGroveTOML, Content: grove, Mode: 0o644})
	}

	if machine, err := s.renderMachineTOML(); err != nil {
		return nil, err
	} else if machine != "" {
		files = append(files, SeedFile{Name: SeedFileMachineTOML, Content: machine, Mode: 0o644})
	}

	// Render and validate the recorded pair together. Notebooks precede roots
	// so applying a seed never exposes roots whose bindings are unresolved.
	notebooks, roots, err := s.renderRecordedPair()
	if err != nil {
		return nil, err
	}
	if notebooks != "" {
		files = append(files, SeedFile{Name: SeedFileNotebooksTOML, Content: notebooks, Mode: 0o644})
	}
	if roots != "" {
		files = append(files, SeedFile{Name: SeedFileRootsTOML, Content: roots, Mode: 0o644})
	}

	if sync, err := s.renderSyncTOML(); err != nil {
		return nil, err
	} else if sync != "" {
		// 0600: sync.toml can carry a literal token, and even when it only
		// carries a token_command it names how to get one.
		files = append(files, SeedFile{Name: SeedFileSyncTOML, Content: sync, Mode: 0o600})
	}

	for _, f := range files {
		if err := validateSeedFileName(f.Name); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// header renders the provenance comment block for one file.
func (s ConfigSeed) header(what string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", what)
	if p := strings.TrimSpace(s.Provenance); p != "" {
		for _, line := range strings.Split(p, "\n") {
			fmt.Fprintf(&b, "# %s\n", line)
		}
	}
	b.WriteString("# Generated — edits are preserved on re-provision only if you move them\n")
	b.WriteString("# into a fragment under conf.d/ (this file is rewritten).\n")
	return b.String()
}

func (s ConfigSeed) renderGroveTOML() (string, error) {
	if len(s.Notebooks) == 0 && !s.DaemonSSH && !(s.LegacyGroves && len(s.Ecosystems) > 0) {
		return "", nil
	}
	var b strings.Builder
	b.WriteString(s.header("Host-level grove configuration."))

	if s.LegacyGroves {
		for _, name := range sortedKeys(s.Ecosystems) {
			eco := s.Ecosystems[name]
			if eco.Path == "" {
				return "", fmt.Errorf("config seed: ecosystem %q has no path", name)
			}
			fmt.Fprintf(&b, "\n[groves.%s]\n", tomlKey(name))
			fmt.Fprintf(&b, "path = %s\n", strconv.Quote(eco.Path))
			if eco.Notebook != "" {
				fmt.Fprintf(&b, "notebook = %s\n", strconv.Quote(eco.Notebook))
			}
			if eco.Description != "" {
				fmt.Fprintf(&b, "description = %s\n", strconv.Quote(eco.Description))
			}
			enabled := eco.Enabled == nil || *eco.Enabled
			fmt.Fprintf(&b, "enabled = %t\n", enabled)
		}
	}

	for _, name := range sortedKeys(s.Notebooks) {
		root := s.Notebooks[name]
		if strings.TrimSpace(root) == "" {
			return "", fmt.Errorf("config seed: notebook %q has an empty root_dir", name)
		}
		fmt.Fprintf(&b, "\n[notebooks.definitions.%s]\n", tomlKey(name))
		fmt.Fprintf(&b, "root_dir = %s\n", strconv.Quote(root))
	}

	if s.DaemonSSH {
		b.WriteString("\n[daemon.ssh]\nenabled = true\n")
	}

	content := b.String()
	// Parse back through the real file decoder without consulting this host's
	// machine-local routing files: seed validation must be self-contained.
	if _, err := unmarshalConfig(SeedFileGroveTOML, []byte(content)); err != nil {
		return "", fmt.Errorf("config seed: generated %s does not load: %w", SeedFileGroveTOML, err)
	}
	return content, nil
}

func (s ConfigSeed) renderMachineTOML() (string, error) {
	if s.MachineName == "" && len(s.Ecosystems) == 0 && len(s.Roots) == 0 {
		return "", nil
	}
	probe := MachineConfig{Machine: MachineSettings{
		Name:       s.MachineName,
		Ecosystems: s.Ecosystems,
		Roots:      s.Roots,
	}}
	if err := probe.Validate(); err != nil {
		return "", fmt.Errorf("config seed: %w", err)
	}

	var b strings.Builder
	b.WriteString(s.header("This machine's grove intent — name, subscriptions, bare roots."))
	b.WriteString("# Dotfiles-portable on purpose: the identity ULID lives in state, not here.\n")

	if s.MachineName != "" {
		fmt.Fprintf(&b, "\n[machine]\nname = %s\n", strconv.Quote(s.MachineName))
	}
	for _, name := range sortedKeys(s.Ecosystems) {
		b.WriteString("\n")
		b.WriteString(renderMachineEcosystem(name, s.Ecosystems[name]))
	}
	for _, name := range sortedKeys(s.Roots) {
		b.WriteString("\n")
		b.WriteString(renderMachineRoot(name, s.Roots[name]))
	}

	content := b.String()
	if _, err := ParseMachineConfigContent(SeedFileMachineTOML, content); err != nil {
		return "", fmt.Errorf("config seed: generated %s does not load: %w", SeedFileMachineTOML, err)
	}
	return content, nil
}

func (s ConfigSeed) renderRecordedPair() (notebooks, roots string, err error) {
	if len(s.CodeRoots) == 0 && len(s.RecordedNotebooks) == 0 && s.RecordedDefaultNotebook == "" {
		return "", "", nil
	}

	if len(s.RecordedNotebooks) > 0 || s.RecordedDefaultNotebook != "" {
		body, marshalErr := toml.Marshal(coderoot.NotebooksFile{
			Default: s.RecordedDefaultNotebook, Notebooks: s.RecordedNotebooks,
		})
		if marshalErr != nil {
			return "", "", fmt.Errorf("config seed: render %s: %w", SeedFileNotebooksTOML, marshalErr)
		}
		notebooks = s.header("Recorded notebook roots and default routing.") + string(body)
	}
	if len(s.CodeRoots) > 0 {
		body, marshalErr := toml.Marshal(coderoot.RootsFile{Roots: s.CodeRoots})
		if marshalErr != nil {
			return "", "", fmt.Errorf("config seed: render %s: %w", SeedFileRootsTOML, marshalErr)
		}
		roots = s.header("Recorded code roots and notebook bindings.") + string(body)
	}

	table := coderoot.Table{Roots: map[string]coderoot.Root{}, Notebooks: map[string]coderoot.Notebook{}}
	if roots != "" {
		rf, parseErr := coderoot.ParseRoots(SeedFileRootsTOML, []byte(roots))
		if parseErr != nil {
			return "", "", fmt.Errorf("config seed: %w", parseErr)
		}
		table.Roots, table.RootsFilePath = rf.Roots, SeedFileRootsTOML
	}
	if notebooks != "" {
		nf, parseErr := coderoot.ParseNotebooks(SeedFileNotebooksTOML, []byte(notebooks))
		if parseErr != nil {
			return "", "", fmt.Errorf("config seed: %w", parseErr)
		}
		table.Notebooks, table.Default, table.NotebooksFilePath = nf.Notebooks, nf.Default, SeedFileNotebooksTOML
	}
	if validateErr := table.Validate(); validateErr != nil {
		return "", "", fmt.Errorf("config seed: recorded routing pair: %w", validateErr)
	}
	return notebooks, roots, nil
}

func (s ConfigSeed) renderSyncTOML() (string, error) {
	if s.Sync == nil {
		return "", nil
	}
	var b strings.Builder
	b.WriteString(s.header("Notebook sync client config."))

	if s.Sync.Server != "" {
		fmt.Fprintf(&b, "server = %s\n", strconv.Quote(s.Sync.Server))
	}
	switch {
	case s.Sync.TokenCommand != "":
		fmt.Fprintf(&b, "token_command = %s\n", strconv.Quote(s.Sync.TokenCommand))
	case s.Sync.Token != "":
		fmt.Fprintf(&b, "token = %s\n", strconv.Quote(s.Sync.Token))
	}

	// The one and only rendering path for [[workspaces]] entries — the same
	// choke point ApplySyncEdit uses, so a pull-enabled entry under a
	// push-only role is refused here exactly as it is there.
	entries, err := RenderSyncWorkspaces(s.Sync.Workspaces)
	if err != nil {
		return "", fmt.Errorf("config seed: %w", err)
	}
	b.WriteString(entries)

	content := b.String()
	if err := verifySyncContent(SeedFileSyncTOML, content, s.Sync.Workspaces); err != nil {
		return "", fmt.Errorf("config seed: %w", err)
	}
	return content, nil
}

// validateSeedFileName enforces the closed set of files a seed may produce.
func validateSeedFileName(name string) error {
	switch name {
	case SeedFileGroveTOML, SeedFileMachineTOML, SeedFileNotebooksTOML, SeedFileRootsTOML, SeedFileSyncTOML:
		return nil
	}
	return fmt.Errorf("config seed: %q is not a seedable config file (allowed: %s, %s, %s, %s, %s)",
		name, SeedFileGroveTOML, SeedFileMachineTOML, SeedFileNotebooksTOML, SeedFileRootsTOML, SeedFileSyncTOML)
}

// validateSeedDir enforces that a seeded directory is home-relative and cannot
// escape. The remote unpacker prefixes these with $HOME, so an absolute path
// or a `..` segment would write outside the target's home.
func validateSeedDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("config seed: empty directory entry")
	}
	if filepath.IsAbs(dir) || strings.HasPrefix(dir, "~") {
		return fmt.Errorf("config seed: directory %q must be relative to the home directory", dir)
	}
	for _, seg := range strings.Split(filepath.ToSlash(dir), "/") {
		if seg == ".." {
			return fmt.Errorf("config seed: directory %q may not contain a `..` segment", dir)
		}
	}
	if strings.ContainsAny(dir, "\n\r") {
		return fmt.Errorf("config seed: directory %q may not contain newlines", dir)
	}
	return nil
}

// Bundle renders the seed as a line-oriented text bundle a remote unpacker can
// apply without any shell interpolation of laptop-supplied values.
//
// Format (every directive line starts with `#!`, which generated TOML never
// does — the renderer rejects any content line that would):
//
//	#!grove-config-seed v1
//	#!dir <home-relative path>
//	#!file <name> <octal mode>
//	<content line>
//	...
//
// The caller appends BundleEnd when the bundle shares a stream with other
// content.
func (s ConfigSeed) Bundle() (string, error) {
	files, err := s.Files()
	if err != nil {
		return "", err
	}
	dirs := append([]string(nil), s.Dirs...)
	sort.Strings(dirs)

	var b strings.Builder
	b.WriteString(BundleVersion)
	b.WriteString("\n")
	for _, dir := range dirs {
		if err := validateSeedDir(dir); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "#!dir %s\n", filepath.ToSlash(dir))
	}
	for _, f := range files {
		fmt.Fprintf(&b, "#!file %s %o\n", f.Name, f.Mode.Perm())
		for _, line := range strings.Split(strings.TrimSuffix(f.Content, "\n"), "\n") {
			if strings.HasPrefix(line, "#!") {
				return "", fmt.Errorf("config seed: %s contains a line starting with `#!`, which the bundle format reserves: %q", f.Name, line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

// ApplyConfigSeed writes a seed into a grove config directory (and creates its
// directories relative to home). It is the local counterpart of Bundle, used
// by tests and by any caller seeding a machine it can reach with a filesystem.
//
// It reports the files it wrote, in render order.
func ApplyConfigSeed(configDir, home string, seed ConfigSeed) ([]string, error) {
	files, err := seed.Files()
	if err != nil {
		return nil, err
	}
	if configDir == "" {
		return nil, fmt.Errorf("config seed: no config directory")
	}
	for _, dir := range seed.Dirs {
		if err := validateSeedDir(dir); err != nil {
			return nil, err
		}
		if home == "" {
			return nil, fmt.Errorf("config seed: directory %q needs a home directory to resolve against", dir)
		}
		if mkErr := os.MkdirAll(filepath.Join(home, filepath.FromSlash(dir)), 0o755); mkErr != nil {
			return nil, mkErr
		}
	}
	if len(files) == 0 {
		return nil, nil
	}
	if mkErr := os.MkdirAll(configDir, 0o755); mkErr != nil {
		return nil, mkErr
	}
	written := make([]string, 0, len(files))
	for _, f := range files {
		path := filepath.Join(configDir, f.Name)
		if wErr := os.WriteFile(path, []byte(f.Content), f.Mode); wErr != nil {
			return written, wErr
		}
		// WriteFile honors the mode only when it creates the file; a re-seed
		// over an existing file would keep the old one.
		if cErr := os.Chmod(path, f.Mode); cErr != nil {
			return written, cErr
		}
		written = append(written, path)
	}
	return written, nil
}
