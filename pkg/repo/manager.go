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
	for _, repo := range manifest.Repositories {
		repos = append(repos, repo)
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
			return manifest, nil
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	if err := json.Unmarshal(data, manifest); err != nil {
		return nil, fmt.Errorf("unmarshaling manifest: %w", err)
	}

	return manifest, nil
}

func (m *Manager) saveManifest(manifest *Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	if err := os.WriteFile(m.manifestPath, data, 0644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	return nil
}