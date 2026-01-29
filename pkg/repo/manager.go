package repo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/util/pathutil"
)

type Manager struct {
	basePath     string
	manifestPath string
	disabled     bool // If true, repo management is disabled by config
	mu           sync.Mutex
}

type WorktreeInfo struct {
	Path      string    `json:"path"`
	Commit    string    `json:"commit"`
	SourceRef string    `json:"source_ref,omitempty"` // The original version string (e.g., "v0.13.0", "main")
	LastUsed  time.Time `json:"last_used"`
}

type RepoInfo struct {
	URL       string                  `json:"url"`
	Shorthand string                  `json:"shorthand,omitempty"`
	BarePath  string                  `json:"bare_path"`
	Worktrees map[string]WorktreeInfo `json:"worktrees,omitempty"` // map[commitHash]WorktreeInfo
}

type AuditInfo struct {
	Status     string    `json:"status"` // "passed", "failed", "not_audited"
	AuditedAt  time.Time `json:"audited_at"`
	ReportPath string    `json:"report_path,omitempty"`
}

type Manifest struct {
	Repositories map[string]RepoInfo  `json:"repositories"` // map[repoURL]RepoInfo
	Audits       map[string]AuditInfo `json:"audits"`       // map[commitHash]AuditInfo
}

// GetCxEcosystemPath returns the path to the cx ecosystem root.
// By default this uses XDG data directory (~/.local/share/grove/cx),
// but it can be configured via context.repos_dir.
// Returns empty string if repo management is disabled (repos_dir set to "").
func GetCxEcosystemPath() (string, error) {
	cfg, err := config.LoadDefault()
	if err != nil {
		// If config can't be loaded, fall back to XDG default
		return getDefaultCxPath()
	}

	// Check if context.repos_dir is configured
	if cfg.Context != nil && cfg.Context.ReposDir != nil {
		if *cfg.Context.ReposDir == "" {
			// Empty string means disabled
			return "", nil
		}
		// Expand the configured path
		expanded, err := pathutil.Expand(*cfg.Context.ReposDir)
		if err != nil {
			return "", fmt.Errorf("expanding repos_dir path: %w", err)
		}
		return expanded, nil
	}

	// Default to XDG data directory
	return getDefaultCxPath()
}

// getDefaultCxPath returns the default cx ecosystem path using XDG conventions.
// Uses XDG_DATA_HOME if set, otherwise ~/.local/share/grove/cx
func getDefaultCxPath() (string, error) {
	// Check XDG_DATA_HOME first
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		return filepath.Join(xdgData, "grove", "cx"), nil
	}

	// Fall back to ~/.local/share/grove/cx
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(homeDir, ".local", "share", "grove", "cx"), nil
}

func NewManager() (*Manager, error) {
	// Get the cx ecosystem path from config
	cxPath, err := GetCxEcosystemPath()
	if err != nil {
		return nil, err
	}

	// If cxPath is empty, repo management is disabled
	if cxPath == "" {
		return &Manager{
			disabled: true,
		}, nil
	}

	basePath := filepath.Join(cxPath, "repos")
	manifestPath := filepath.Join(cxPath, "manifest.json")

	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("creating repos directory: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(manifestPath), 0755); err != nil {
		return nil, fmt.Errorf("creating manifest directory: %w", err)
	}

	return &Manager{
		basePath:     basePath,
		manifestPath: manifestPath,
	}, nil
}

// extractShorthand extracts "owner/repo" from common Git hosting URLs.
// Returns empty string if no shorthand can be extracted.
func extractShorthand(repoURL string) string {
	// Remove protocol prefixes
	url := strings.TrimPrefix(repoURL, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "git@")

	// Replace colon (from git@host:owner/repo) with slash
	url = strings.Replace(url, ":", "/", 1)

	// Check for common hosting providers
	for _, host := range []string{"github.com", "gitlab.com", "bitbucket.com"} {
		if strings.HasPrefix(url, host+"/") {
			// Extract the part after the host
			parts := strings.SplitN(url, "/", 3)
			if len(parts) >= 3 {
				shorthand := parts[1] + "/" + parts[2]
				// Remove .git suffix if present
				shorthand = strings.TrimSuffix(shorthand, ".git")
				return shorthand
			}
		}
	}

	return ""
}

// Ensure makes sure the bare clone for the given repository exists and is up-to-date.
// It does not perform any checkouts. Use EnsureVersion for version-specific worktrees.
// Returns an error if repo management is disabled by config.
func (m *Manager) Ensure(repoURL string) error {
	if m.disabled {
		return fmt.Errorf("repository management is disabled by config (context.repos_dir is empty)")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, err := m.loadManifest()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	barePath := m.getLocalPath(repoURL)

	// Check if bare repository exists
	if _, err := os.Stat(barePath); os.IsNotExist(err) {
		// Clone as bare repository
		if err := m.cloneRepository(repoURL, barePath); err != nil {
			return fmt.Errorf("cloning repository: %w", err)
		}
	} else {
		// Fetch updates for existing bare repository
		if err := m.fetchRepository(barePath); err != nil {
			return fmt.Errorf("fetching repository: %w", err)
		}
	}

	// Update manifest
	if manifest.Repositories == nil {
		manifest.Repositories = make(map[string]RepoInfo)
	}

	info := RepoInfo{
		URL:       repoURL,
		Shorthand: extractShorthand(repoURL),
		BarePath:  barePath,
	}

	manifest.Repositories[repoURL] = info

	if err := m.saveManifest(manifest); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	return nil
}

// EnsureVersion ensures a specific version of a repository is checked out in a worktree.
// It returns the absolute path to the worktree and the resolved commit hash.
func (m *Manager) EnsureVersion(repoURL, version string) (worktreePath string, resolvedCommit string, err error) {
	// Ensure the bare clone exists and is up-to-date
	if err := m.Ensure(repoURL); err != nil {
		return "", "", fmt.Errorf("ensuring bare clone: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, err := m.loadManifest()
	if err != nil {
		return "", "", fmt.Errorf("loading manifest: %w", err)
	}

	info, exists := manifest.Repositories[repoURL]
	if !exists {
		return "", "", fmt.Errorf("repository %s not found in manifest after ensure", repoURL)
	}

	barePath := info.BarePath

	// Resolve the version to a commit hash
	// If version is empty, use origin/HEAD (default branch)
	versionToResolve := version
	if versionToResolve == "" {
		versionToResolve = "origin/HEAD"
	}

	resolvedCommit, err = m.resolveVersion(barePath, versionToResolve)
	if err != nil {
		return "", "", fmt.Errorf("resolving version %s: %w", versionToResolve, err)
	}

	// Create worktree under the bare repo in .grove-worktrees/{commit-hash}
	// This follows the standard workspace pattern
	worktreeDir := filepath.Join(barePath, ".grove-worktrees")
	worktreePath = filepath.Join(worktreeDir, resolvedCommit[:12])

	// Create worktree directory if needed
	if err := os.MkdirAll(worktreeDir, 0755); err != nil {
		return "", "", fmt.Errorf("creating worktree directory: %w", err)
	}

	// Check if worktree already exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		// Create the worktree
		if err := m.createWorktree(barePath, worktreePath, resolvedCommit); err != nil {
			return "", "", fmt.Errorf("creating worktree: %w", err)
		}
	}

	// Update manifest with worktree information
	if info.Worktrees == nil {
		info.Worktrees = make(map[string]WorktreeInfo)
	}

	// Store the worktree info with the original version as SourceRef
	info.Worktrees[resolvedCommit] = WorktreeInfo{
		Path:      worktreePath,
		Commit:    resolvedCommit,
		SourceRef: version, // Store the original version string
		LastUsed:  time.Now(),
	}

	manifest.Repositories[repoURL] = info

	if err := m.saveManifest(manifest); err != nil {
		return "", "", fmt.Errorf("saving manifest with worktree info: %w", err)
	}

	return worktreePath, resolvedCommit, nil
}

// resolveVersion resolves a version string (branch, tag, or commit) to a full commit hash
func (m *Manager) resolveVersion(barePath, version string) (string, error) {
	// Special handling for origin/HEAD or empty version - find the default branch
	if version == "origin/HEAD" || version == "" {
		// Try to get the symbolic ref for origin/HEAD
		cmd := exec.Command("git", "-C", barePath, "symbolic-ref", "refs/remotes/origin/HEAD")
		if output, err := cmd.Output(); err == nil {
			// Parse the ref (e.g., "refs/remotes/origin/main" -> "origin/main")
			ref := strings.TrimSpace(string(output))
			if strings.HasPrefix(ref, "refs/remotes/origin/") {
				version = "origin/" + strings.TrimPrefix(ref, "refs/remotes/origin/")
			}
		} else {
			// Fallback: try common default branch names
			for _, branch := range []string{"origin/main", "origin/master"} {
				cmd := exec.Command("git", "-C", barePath, "rev-parse", branch+"^{commit}")
				if output, err := cmd.Output(); err == nil {
					return strings.TrimSpace(string(output)), nil
				}
			}
			return "", fmt.Errorf("could not determine default branch")
		}
	}

	// Try the version as-is first (could be a tag, commit hash, or already-prefixed branch)
	cmd := exec.Command("git", "-C", barePath, "rev-parse", version+"^{commit}")
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output)), nil
	}

	// For bare repos, branches are in refs/heads/, not refs/remotes/origin/
	// Try various common patterns
	if !strings.Contains(version, "/") {
		candidates := []string{
			"refs/heads/" + version,    // bare repo branches
			"origin/" + version,         // regular clone remote branches
			"refs/remotes/origin/" + version, // full remote ref
		}

		for _, candidate := range candidates {
			cmd = exec.Command("git", "-C", barePath, "rev-parse", candidate+"^{commit}")
			output, err = cmd.Output()
			if err == nil {
				return strings.TrimSpace(string(output)), nil
			}
		}
	}

	// Resolution failed - try to provide helpful suggestions
	cmd = exec.Command("git", "-C", barePath, "for-each-ref", "--format=%(refname:short)", "refs/heads/", "refs/tags/")
	if output, err := cmd.Output(); err == nil {
		refs := strings.TrimSpace(string(output))
		if refs != "" {
			lines := strings.Split(refs, "\n")
			// Limit to first 5 suggestions
			if len(lines) > 5 {
				lines = lines[:5]
			}
			return "", fmt.Errorf("could not resolve version '%s'. Available refs: %s", version, strings.Join(lines, ", "))
		}
	}
	return "", fmt.Errorf("could not resolve version '%s' (tried as tag, commit, and branch)", version)
}

// createWorktree creates a new worktree at the specified path for the given commit
func (m *Manager) createWorktree(barePath, worktreePath, commitHash string) error {
	cmd := exec.Command("git", "-C", barePath, "worktree", "add", worktreePath, commitHash)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// IsDisabled returns true if repository management is disabled by config.
func (m *Manager) IsDisabled() bool {
	return m.disabled
}

func (m *Manager) List() ([]RepoInfo, error) {
	// If disabled, return empty list (not an error)
	if m.disabled {
		return []RepoInfo{}, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, err := m.loadManifest()
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}

	var repos []RepoInfo
	needsSave := false
	for url, repo := range manifest.Repositories {
		// Migrate old repos: populate BarePath if missing
		if repo.BarePath == "" {
			repo.BarePath = m.getLocalPath(url)
			manifest.Repositories[url] = repo
			needsSave = true
		}
		repos = append(repos, repo)
	}

	// Save manifest if we migrated any repos
	if needsSave {
		if err := m.saveManifest(manifest); err != nil {
			return nil, fmt.Errorf("saving migrated manifest: %w", err)
		}
	}

	return repos, nil
}

func (m *Manager) Sync() error {
	if m.disabled {
		return fmt.Errorf("repository management is disabled by config (context.repos_dir is empty)")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, err := m.loadManifest()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	for url, info := range manifest.Repositories {
		// Fetch latest changes for each bare repository
		if err := m.fetchRepository(info.BarePath); err != nil {
			return fmt.Errorf("fetching %s: %w", url, err)
		}
	}

	return nil
}

// UpdateAuditResult updates the audit status for a specific commit hash
func (m *Manager) UpdateAuditResult(commitHash, status, reportPath string) error {
	if m.disabled {
		return fmt.Errorf("repository management is disabled by config (context.repos_dir is empty)")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, err := m.loadManifest()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	if manifest.Audits == nil {
		manifest.Audits = make(map[string]AuditInfo)
	}

	manifest.Audits[commitHash] = AuditInfo{
		Status:     status,
		AuditedAt:  time.Now(),
		ReportPath: reportPath,
	}

	return m.saveManifest(manifest)
}

// GetAuditInfo returns the audit information for a specific commit hash
func (m *Manager) GetAuditInfo(commitHash string) (AuditInfo, bool) {
	if m.disabled {
		return AuditInfo{}, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, err := m.loadManifest()
	if err != nil {
		return AuditInfo{}, false
	}

	info, exists := manifest.Audits[commitHash]
	return info, exists
}

func (m *Manager) getLocalPath(repoURL string) string {
	url := strings.TrimPrefix(repoURL, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimSuffix(url, ".git")
	
	hash := sha256.Sum256([]byte(repoURL))
	hashStr := hex.EncodeToString(hash[:])[:8]
	
	safePath := strings.ReplaceAll(url, "/", "_")
	dirName := fmt.Sprintf("%s_%s", safePath, hashStr)
	
	return filepath.Join(m.basePath, dirName)
}

func (m *Manager) cloneRepository(repoURL, barePath string) error {
	cmd := exec.Command("git", "clone", "--bare", repoURL, barePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone --bare failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func (m *Manager) fetchRepository(localPath string) error {
	cmd := exec.Command("git", "-C", localPath, "fetch", "--all", "--prune")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git fetch failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}


func (m *Manager) LoadManifest() (*Manifest, error) {
	if m.disabled {
		return &Manifest{Repositories: make(map[string]RepoInfo)}, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadManifest()
}

func (m *Manager) loadManifest() (*Manifest, error) {
	manifest := &Manifest{
		Repositories: make(map[string]RepoInfo),
	}

	data, err := os.ReadFile(m.manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Main manifest doesn't exist, try loading from backup.
			return m.loadFromBackup(manifest)
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	if err := json.Unmarshal(data, manifest); err != nil {
		// Unmarshal failed, indicating corruption. Try the backup.
		fmt.Fprintf(os.Stderr, "Warning: manifest file %s is corrupt. Attempting to recover from backup.\n", m.manifestPath)
		recoveredManifest, backupErr := m.loadFromBackup(manifest)
		if backupErr != nil {
			// Backup failed too. Return the original corruption error, but add context.
			return nil, fmt.Errorf("unmarshaling manifest failed and backup could not be loaded: %w; backup error: %v", err, backupErr)
		}
		// Recovery successful.
		return recoveredManifest, nil
	}

	return manifest, nil
}

// loadFromBackup attempts to load the manifest from the backup file.
// If successful, it restores the main manifest file from the backup.
func (m *Manager) loadFromBackup(originalManifest *Manifest) (*Manifest, error) {
	backupPath := m.manifestPath + ".bak"
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No main file and no backup, just return the empty manifest.
			return originalManifest, nil
		}
		return nil, fmt.Errorf("reading backup manifest: %w", err)
	}

	// Unmarshal the backup data into a new manifest struct to avoid side effects.
	backupManifest := &Manifest{
		Repositories: make(map[string]RepoInfo),
	}
	if err := json.Unmarshal(backupData, backupManifest); err != nil {
		return nil, fmt.Errorf("unmarshaling backup manifest: %w", err)
	}

	// Restore the main manifest from the backup
	if err := os.WriteFile(m.manifestPath, backupData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to restore manifest from backup: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Successfully recovered manifest from backup %s.\n", backupPath)
	}

	return backupManifest, nil
}

func (m *Manager) saveManifest(manifest *Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	// 1. Write to a temporary file.
	manifestDir := filepath.Dir(m.manifestPath)
	tempFile, err := os.CreateTemp(manifestDir, "manifest-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp manifest file: %w", err)
	}

	successful := false
	defer func() {
		if !successful {
			os.Remove(tempFile.Name())
		}
	}()

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return fmt.Errorf("writing to temp manifest file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("closing temp manifest file: %w", err)
	}

	// 2. Backup the existing manifest file if it exists.
	backupPath := m.manifestPath + ".bak"
	if _, err := os.Stat(m.manifestPath); err == nil {
		if err := os.Rename(m.manifestPath, backupPath); err != nil {
			return fmt.Errorf("failed to backup manifest: %w", err)
		}
	}

	// 3. Atomically rename the temporary file to the final path.
	if err := os.Rename(tempFile.Name(), m.manifestPath); err != nil {
		// If the final rename fails, try to restore the backup.
		if _, backupErr := os.Stat(backupPath); backupErr == nil {
			os.Rename(backupPath, m.manifestPath)
		}
		return fmt.Errorf("failed to activate new manifest: %w", err)
	}

	successful = true
	return nil
}