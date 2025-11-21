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
)

type Manager struct {
	basePath     string
	manifestPath string
	mu           sync.Mutex
}

type RepoInfo struct {
	URL            string    `json:"url"`
	Shorthand      string    `json:"shorthand,omitempty"`
	LocalPath      string    `json:"local_path"`
	PinnedVersion  string    `json:"pinned_version"`
	ResolvedCommit string    `json:"resolved_commit"`
	LastSyncedAt   time.Time `json:"last_synced_at"`
	Audit          AuditInfo `json:"audit"`
}

type AuditInfo struct {
	Status        string    `json:"status"`
	AuditedAt     time.Time `json:"audited_at"`
	AuditedCommit string    `json:"audited_commit"`
	ReportPath    string    `json:"report_path,omitempty"`
}

type Manifest struct {
	Repositories map[string]RepoInfo `json:"repositories"`
}

func NewManager() (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}

	basePath := filepath.Join(homeDir, ".grove", "cx", "repos")
	manifestPath := filepath.Join(homeDir, ".grove", "cx", "manifest.json")

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

func (m *Manager) Ensure(repoURL, version string) (localPath string, resolvedCommit string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, err := m.loadManifest()
	if err != nil {
		return "", "", fmt.Errorf("loading manifest: %w", err)
	}

	localPath = m.getLocalPath(repoURL)

	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		if err := m.cloneRepository(repoURL, localPath); err != nil {
			return "", "", fmt.Errorf("cloning repository: %w", err)
		}
	} else {
		if err := m.fetchRepository(localPath); err != nil {
			return "", "", fmt.Errorf("fetching repository: %w", err)
		}
	}

	if version != "" {
		if err := m.checkoutVersion(localPath, version); err != nil {
			return "", "", fmt.Errorf("checking out version %s: %w", version, err)
		}
	}

	resolvedCommit, err = m.getResolvedCommit(localPath)
	if err != nil {
		return "", "", fmt.Errorf("getting resolved commit: %w", err)
	}

	info := RepoInfo{
		URL:            repoURL,
		Shorthand:      extractShorthand(repoURL),
		LocalPath:      localPath,
		PinnedVersion:  version,
		ResolvedCommit: resolvedCommit,
		LastSyncedAt:   time.Now(),
		Audit: AuditInfo{
			Status: "not_audited",
		},
	}

	if manifest.Repositories == nil {
		manifest.Repositories = make(map[string]RepoInfo)
	}
	manifest.Repositories[repoURL] = info

	if err := m.saveManifest(manifest); err != nil {
		return "", "", fmt.Errorf("saving manifest: %w", err)
	}

	return localPath, resolvedCommit, nil
}

func (m *Manager) List() ([]RepoInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, err := m.loadManifest()
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}

	var repos []RepoInfo
	needsSave := false
	for url, repo := range manifest.Repositories {
		// Populate shorthand if not already set (for backwards compatibility)
		if repo.Shorthand == "" {
			repo.Shorthand = extractShorthand(url)
			manifest.Repositories[url] = repo
			needsSave = true
		}
		repos = append(repos, repo)
	}

	// Save the manifest if we backfilled any shorthands
	if needsSave {
		if err := m.saveManifest(manifest); err != nil {
			// Don't fail the list operation if save fails
			fmt.Fprintf(os.Stderr, "Warning: failed to save backfilled shorthands: %v\n", err)
		}
	}

	return repos, nil
}

func (m *Manager) Sync() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, err := m.loadManifest()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	for url, info := range manifest.Repositories {
		if err := m.fetchRepository(info.LocalPath); err != nil {
			return fmt.Errorf("fetching %s: %w", url, err)
		}

		if info.PinnedVersion != "" {
			if err := m.checkoutVersion(info.LocalPath, info.PinnedVersion); err != nil {
				return fmt.Errorf("checking out %s for %s: %w", info.PinnedVersion, url, err)
			}
		}

		resolvedCommit, err := m.getResolvedCommit(info.LocalPath)
		if err != nil {
			return fmt.Errorf("getting resolved commit for %s: %w", url, err)
		}

		info.ResolvedCommit = resolvedCommit
		info.LastSyncedAt = time.Now()
		// Populate shorthand if not already set (for backwards compatibility)
		if info.Shorthand == "" {
			info.Shorthand = extractShorthand(url)
		}
		manifest.Repositories[url] = info
	}

	return m.saveManifest(manifest)
}

func (m *Manager) UpdateAuditStatus(repoURL, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, err := m.loadManifest()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	info, exists := manifest.Repositories[repoURL]
	if !exists {
		return fmt.Errorf("repository %s not found in manifest", repoURL)
	}

	info.Audit.Status = status
	info.Audit.AuditedAt = time.Now()
	info.Audit.AuditedCommit = info.ResolvedCommit
	manifest.Repositories[repoURL] = info

	return m.saveManifest(manifest)
}

func (m *Manager) UpdateAuditResult(repoURL, status, reportPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, err := m.loadManifest()
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	info, exists := manifest.Repositories[repoURL]
	if !exists {
		return fmt.Errorf("repository %s not found in manifest", repoURL)
	}

	info.Audit.Status = status
	info.Audit.AuditedAt = time.Now()
	info.Audit.AuditedCommit = info.ResolvedCommit
	info.Audit.ReportPath = reportPath
	manifest.Repositories[repoURL] = info

	return m.saveManifest(manifest)
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

func (m *Manager) cloneRepository(repoURL, localPath string) error {
	cmd := exec.Command("git", "clone", repoURL, localPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(output))
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

func (m *Manager) checkoutVersion(localPath, version string) error {
	cmd := exec.Command("git", "-C", localPath, "checkout", version)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func (m *Manager) getResolvedCommit(localPath string) (string, error) {
	cmd := exec.Command("git", "-C", localPath, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (m *Manager) LoadManifest() (*Manifest, error) {
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