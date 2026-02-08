package sessions

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	coreconfig "github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/process"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/util/frontmatter"
	"github.com/sirupsen/logrus"
)

// DiscoverLiveSessions scans ~/.grove/hooks/sessions/ directory and returns live sessions.
// A session is considered live if its PID is still alive.
// This is a core-only implementation without storage dependencies.
func DiscoverLiveSessions() ([]*models.Session, error) {
	groveSessionsDir := filepath.Join(paths.StateDir(), "hooks", "sessions")

	if _, err := os.Stat(groveSessionsDir); os.IsNotExist(err) {
		return []*models.Session{}, nil
	}

	entries, err := os.ReadDir(groveSessionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []*models.Session

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		sessionDir := filepath.Join(groveSessionsDir, dirName)
		pidFile := filepath.Join(sessionDir, "pid.lock")
		metadataFile := filepath.Join(sessionDir, "metadata.json")

		// Read PID
		pidContent, err := os.ReadFile(pidFile)
		if err != nil {
			continue
		}

		var pid int
		if _, err := fmt.Sscanf(string(pidContent), "%d", &pid); err != nil {
			continue
		}

		// Check if process is alive
		isAlive := process.IsProcessAlive(pid)

		// Read metadata
		metadataContent, err := os.ReadFile(metadataFile)
		if err != nil {
			continue
		}

		var metadata SessionMetadata
		if err := json.Unmarshal(metadataContent, &metadata); err != nil {
			continue
		}

		sessionID := metadata.SessionID
		claudeSessionID := metadata.ClaudeSessionID
		if claudeSessionID == "" {
			claudeSessionID = dirName
		}

		status := "running"
		var endedAt *time.Time
		lastActivity := metadata.StartedAt

		if !isAlive {
			status = "interrupted"
			now := time.Now()
			endedAt = &now
			lastActivity = now
		}

		session := &models.Session{
			ID:               sessionID,
			Type:             metadata.Type,
			ClaudeSessionID:  claudeSessionID,
			PID:              pid,
			Repo:             metadata.Repo,
			Branch:           metadata.Branch,
			WorkingDirectory: metadata.WorkingDirectory,
			User:             metadata.User,
			Status:           status,
			StartedAt:        metadata.StartedAt,
			LastActivity:     lastActivity,
			EndedAt:          endedAt,
			IsTest:           false,
			JobTitle:         metadata.JobTitle,
			PlanName:         metadata.PlanName,
			JobFilePath:      metadata.JobFilePath,
			Provider:         metadata.Provider,
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

// DiscoverJobsFromFiles scans directories for job files and returns sessions.
// This uses the lightweight frontmatter parser to avoid importing flow packages.
func DiscoverJobsFromFiles(dirs []string) ([]*models.Session, error) {
	var sessions []*models.Session
	seen := make(map[string]bool)

	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			// Skip archive directories
			if info.IsDir() {
				name := info.Name()
				if name == "archive" || name == ".archive" ||
					strings.HasPrefix(name, "archive-") || strings.HasPrefix(name, ".archive-") {
					return filepath.SkipDir
				}
				return nil
			}

			if !strings.HasSuffix(info.Name(), ".md") {
				return nil
			}
			if info.Name() == "spec.md" || info.Name() == "README.md" {
				return nil
			}

			if seen[path] {
				return nil
			}

			session := parseJobFileToSession(path)
			if session != nil {
				seen[path] = true
				sessions = append(sessions, session)
			}

			return nil
		})
		if err != nil {
			continue
		}
	}

	return sessions, nil
}

// parseJobFileToSession parses a job markdown file and returns a Session.
func parseJobFileToSession(path string) *models.Session {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	meta, err := frontmatter.Parse(file)
	if err != nil {
		return nil
	}

	// Only process valid job types or active statuses
	isActiveStatus := meta.Status == "running" || meta.Status == "pending_user" || meta.Status == "idle"
	isValidJobType := meta.Type == "chat" || meta.Type == "oneshot" || meta.Type == "agent" ||
		meta.Type == "interactive_agent" || meta.Type == "headless_agent" || meta.Type == "shell"

	if !isValidJobType && !isActiveStatus {
		return nil
	}

	sessionType := meta.Type
	if sessionType == "" && isActiveStatus {
		sessionType = "note"
	}

	// Determine repo and branch from path
	pathParts := strings.Split(path, string(filepath.Separator))
	var repo, branch string
	for i, part := range pathParts {
		if part == "repos" && i+2 < len(pathParts) {
			repo = pathParts[i+1]
			branch = pathParts[i+2]
			break
		} else if part == ".grove-worktrees" && i > 0 && i+1 < len(pathParts) {
			repo = pathParts[i-1]
			branch = pathParts[i+1]
			break
		}
	}

	planName := filepath.Base(filepath.Dir(path))

	// Determine effective status based on lock file and PID
	status := meta.Status
	if status == "running" || status == "pending_user" {
		// Check lock file for non-chat/non-interactive_agent jobs
		if meta.Type != "chat" && meta.Type != "interactive_agent" {
			lockFile := path + ".lock"
			pidContent, err := os.ReadFile(lockFile)
			if err != nil {
				status = "interrupted"
			} else {
				var pid int
				if _, err := fmt.Sscanf(string(pidContent), "%d", &pid); err == nil {
					if !process.IsProcessAlive(pid) {
						status = "interrupted"
					}
				} else {
					status = "interrupted"
				}
			}
		}
	}

	session := &models.Session{
		ID:           meta.ID,
		Type:         sessionType,
		Status:       status,
		Repo:         repo,
		Branch:       branch,
		StartedAt:    meta.StartedAt,
		LastActivity: meta.UpdatedAt,
		PlanName:     planName,
		JobTitle:     meta.Title,
		JobFilePath:  path,
	}

	return session
}

// GetJobStatus reads the current status of a job file.
func GetJobStatus(jobFilePath string) (string, error) {
	file, err := os.Open(jobFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to open job file: %w", err)
	}
	defer file.Close()

	meta, err := frontmatter.Parse(file)
	if err != nil {
		return "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Terminal states are authoritative
	terminalStates := map[string]bool{
		"completed": true, "failed": true, "interrupted": true,
		"error": true, "abandoned": true, "hold": true, "todo": true, "pending": true,
	}
	if terminalStates[meta.Status] {
		return meta.Status, nil
	}

	// For running/pending_user states, verify with lock file
	if meta.Status == "running" || meta.Status == "pending_user" {
		if meta.Type == "chat" || meta.Type == "interactive_agent" {
			return meta.Status, nil
		}

		lockFile := jobFilePath + ".lock"
		pidContent, err := os.ReadFile(lockFile)
		if err != nil {
			return "interrupted", nil
		}

		var pid int
		if _, err := fmt.Sscanf(string(pidContent), "%d", &pid); err != nil {
			return "interrupted", nil
		}

		if !process.IsProcessAlive(pid) {
			return "interrupted", nil
		}

		return "running", nil
	}

	return meta.Status, nil
}

// DiscoverAll returns all active sessions from all sources (Interactive, Flow Jobs, OpenCode).
// This is the main entry point for LocalClient and can be used as a complete fallback
// when the daemon is not available.
func DiscoverAll() ([]*models.Session, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allSessions []*models.Session

	// 1. Interactive Sessions (Hook-based)
	wg.Add(1)
	go func() {
		defer wg.Done()
		sessions, err := DiscoverLiveSessions()
		if err != nil {
			return
		}
		mu.Lock()
		allSessions = append(allSessions, sessions...)
		mu.Unlock()
	}()

	// 2. Flow Jobs (Markdown files)
	wg.Add(1)
	go func() {
		defer wg.Done()
		sessions, err := DiscoverFlowJobs()
		if err != nil {
			return
		}
		mu.Lock()
		allSessions = append(allSessions, sessions...)
		mu.Unlock()
	}()

	// 3. OpenCode Sessions
	wg.Add(1)
	go func() {
		defer wg.Done()
		sessions, err := DiscoverOpenCodeSessions()
		if err != nil {
			return
		}
		mu.Lock()
		allSessions = append(allSessions, sessions...)
		mu.Unlock()
	}()

	wg.Wait()

	// Deduplicate by ID, prioritizing Interactive sessions (which have PIDs)
	sessionMap := make(map[string]*models.Session)
	for _, s := range allSessions {
		if existing, ok := sessionMap[s.ID]; ok {
			// Merge logic: If existing has PID and new doesn't, keep existing
			if existing.PID > 0 && s.PID == 0 {
				// Enrich existing with missing data from new
				if existing.Provider == "" && s.Provider != "" {
					existing.Provider = s.Provider
				}
				continue
			}
			// If new has PID and existing doesn't, prefer new
			if s.PID > 0 && existing.PID == 0 {
				// Enrich new with missing data from existing
				if s.Provider == "" && existing.Provider != "" {
					s.Provider = existing.Provider
				}
				sessionMap[s.ID] = s
				continue
			}
			// Both have or both lack PID - prefer the one with more info
			if existing.Provider == "" && s.Provider != "" {
				existing.Provider = s.Provider
			}
		} else {
			sessionMap[s.ID] = s
		}
	}

	result := make([]*models.Session, 0, len(sessionMap))
	for _, s := range sessionMap {
		result = append(result, s)
	}

	// Sort by last activity (most recent first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastActivity.After(result[j].LastActivity)
	})

	return result, nil
}

// DiscoverFlowJobs scans all workspace directories for job files (plans, chats, notes).
// This is the core-level implementation that mirrors the hooks discovery logic
// but without caching (caching is handled at the daemon level).
//
// NOTE: This function performs a full workspace discovery which is expensive.
// For daemon use, prefer DiscoverFlowJobsWithNodes which reuses cached workspace data.
func DiscoverFlowJobs() ([]*models.Session, error) {
	// Load configuration
	coreCfg, err := coreconfig.LoadDefault()
	if err != nil {
		coreCfg = &coreconfig.Config{} // Proceed with defaults on error
	}

	// Initialize workspace provider
	logger := logrus.New()
	logger.SetOutput(io.Discard) // Suppress discovery debug output
	discoveryService := workspace.NewDiscoveryService(logger)
	discoveryResult, err := discoveryService.DiscoverAll()
	if err != nil {
		return nil, fmt.Errorf("workspace discovery failed: %w", err)
	}
	provider := workspace.NewProvider(discoveryResult)

	return DiscoverFlowJobsWithProvider(provider, coreCfg)
}

// DiscoverFlowJobsWithNodes scans workspace directories for job files using pre-built nodes.
// This is much more efficient than DiscoverFlowJobs as it avoids the expensive workspace
// discovery step. Use this when you already have workspace data (e.g., from daemon cache).
func DiscoverFlowJobsWithNodes(nodes []*workspace.WorkspaceNode) ([]*models.Session, error) {
	// Load configuration
	coreCfg, err := coreconfig.LoadDefault()
	if err != nil {
		coreCfg = &coreconfig.Config{} // Proceed with defaults on error
	}

	provider := workspace.NewProviderFromNodes(nodes)
	return DiscoverFlowJobsWithProvider(provider, coreCfg)
}

// DiscoverFlowJobsWithProvider scans workspace directories for job files using an existing provider.
// This is the core implementation shared by DiscoverFlowJobs and DiscoverFlowJobsWithNodes.
func DiscoverFlowJobsWithProvider(provider *workspace.Provider, coreCfg *coreconfig.Config) ([]*models.Session, error) {
	locator := workspace.NewNotebookLocator(coreCfg)

	// Scan for all plan, chat, and note directories
	planDirs, _ := locator.ScanForAllPlans(provider)
	chatDirs, _ := locator.ScanForAllChats(provider)
	noteDirs, _ := locator.ScanForAllNotes(provider)

	allScanDirs := append(planDirs, chatDirs...)
	allScanDirs = append(allScanDirs, noteDirs...)

	// Walk directories and collect job files
	var sessions []*models.Session
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 20) // Limit concurrency

	for _, scanDir := range allScanDirs {
		dirPath := scanDir.Path
		ownerNode := scanDir.Owner

		wg.Add(1)
		go func(path string, owner *workspace.WorkspaceNode) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			_ = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}

				// Skip archive directories
				if info.IsDir() {
					name := info.Name()
					if name == "archive" || name == ".archive" ||
						strings.HasPrefix(name, "archive-") || strings.HasPrefix(name, ".archive-") {
						return filepath.SkipDir
					}
					return nil
				}

				if !strings.HasSuffix(info.Name(), ".md") {
					return nil
				}
				if info.Name() == "spec.md" || info.Name() == "README.md" {
					return nil
				}

				session := parseFlowJobFile(filePath, owner, provider)
				if session != nil {
					mu.Lock()
					sessions = append(sessions, session)
					mu.Unlock()
				}
				return nil
			})
		}(dirPath, ownerNode)
	}
	wg.Wait()

	return sessions, nil
}

// genericNoteGroups are standard note type directories that should be associated
// with the parent repository rather than worktrees when discovered.
var genericNoteGroups = map[string]bool{
	"inbox": true, "current": true, "llm": true, "learn": true,
	"daily": true, "issues": true, "architecture": true, "todos": true,
	"quick": true, "archive": true, "prompts": true, "blog": true,
}

// parseFlowJobFile parses a job markdown file and returns a Session.
// This includes the full worktree resolution logic from hooks.
func parseFlowJobFile(path string, ownerNode *workspace.WorkspaceNode, provider *workspace.Provider) *models.Session {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	meta, err := frontmatter.Parse(file)
	if err != nil {
		return nil
	}

	// Only process active statuses or valid job types
	isActiveStatus := meta.Status == "running" || meta.Status == "pending_user" || meta.Status == "idle"
	isValidJobType := meta.Type == "chat" || meta.Type == "oneshot" || meta.Type == "agent" ||
		meta.Type == "interactive_agent" || meta.Type == "headless_agent" || meta.Type == "shell"

	if !isValidJobType && !isActiveStatus {
		return nil
	}

	sessionType := meta.Type
	if sessionType == "" && isActiveStatus {
		sessionType = "note"
	}

	// Determine effective status based on lock file and PID for non-chat jobs
	status := meta.Status
	if status == "running" || status == "pending_user" {
		if meta.Type != "chat" && meta.Type != "interactive_agent" {
			lockFile := path + ".lock"
			pidContent, err := os.ReadFile(lockFile)
			if err != nil {
				status = "interrupted"
			} else {
				var pid int
				if _, err := fmt.Sscanf(string(pidContent), "%d", &pid); err == nil {
					if !process.IsProcessAlive(pid) {
						status = "interrupted"
					}
				} else {
					status = "interrupted"
				}
			}
		}
	}

	// Start with the owner of the plan directory as the default context
	effectiveOwnerNode := ownerNode

	// If the job's frontmatter specifies a worktree, resolve it
	if meta.Worktree != "" {
		resolvedNode := provider.FindByWorktree(ownerNode, meta.Worktree)
		if resolvedNode != nil {
			effectiveOwnerNode = resolvedNode
		}
	}

	// If this is a generic note and its owner is a worktree, re-assign to the parent
	planName := filepath.Base(filepath.Dir(path))
	if genericNoteGroups[planName] && effectiveOwnerNode.IsWorktree() {
		current := effectiveOwnerNode
		for current != nil && current.IsWorktree() {
			if current.ParentProjectPath != "" {
				parentNode := provider.FindByPath(current.ParentProjectPath)
				if parentNode != nil {
					current = parentNode
				} else {
					break
				}
			} else {
				break
			}
		}
		if current != nil && !current.IsWorktree() {
			effectiveOwnerNode = current
		}
	}

	// Determine repo name and worktree
	repoName := effectiveOwnerNode.Name
	worktreeName := ""
	if effectiveOwnerNode.IsWorktree() {
		if effectiveOwnerNode.ParentProjectPath != "" {
			repoName = filepath.Base(effectiveOwnerNode.ParentProjectPath)
		}
		// Handle EcosystemWorktreeSubProjectWorktree
		if string(effectiveOwnerNode.Kind) == "EcosystemWorktreeSubProjectWorktree" {
			worktreeName = filepath.Base(effectiveOwnerNode.ParentEcosystemPath)
		} else {
			worktreeName = effectiveOwnerNode.Name
		}
	}

	return &models.Session{
		ID:               meta.ID,
		Type:             sessionType,
		Status:           status,
		Repo:             repoName,
		Branch:           worktreeName,
		WorkingDirectory: effectiveOwnerNode.Path,
		StartedAt:        meta.StartedAt,
		LastActivity:     meta.UpdatedAt,
		PlanName:         planName,
		JobTitle:         meta.Title,
		JobFilePath:      path,
	}
}

// DiscoverOpenCodeSessions scans ~/.local/share/opencode/storage/ for OpenCode sessions.
func DiscoverOpenCodeSessions() ([]*models.Session, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	storageDir := filepath.Join(homeDir, ".local", "share", "opencode", "storage")
	projectsDir := filepath.Join(storageDir, "project")
	sessionsDir := filepath.Join(storageDir, "session")

	// Check if OpenCode storage exists
	if _, err := os.Stat(storageDir); os.IsNotExist(err) {
		return []*models.Session{}, nil
	}

	// Load all projects to map project IDs to working directories
	projectMap := make(map[string]string) // projectID -> worktree path
	projectEntries, err := os.ReadDir(projectsDir)
	if err == nil {
		for _, entry := range projectEntries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
				projectPath := filepath.Join(projectsDir, entry.Name())
				data, err := os.ReadFile(projectPath)
				if err != nil {
					continue
				}

				var project struct {
					ID       string `json:"id"`
					Worktree string `json:"worktree"`
				}
				if err := json.Unmarshal(data, &project); err != nil {
					continue
				}
				projectMap[project.ID] = project.Worktree
			}
		}
	}

	var sessions []*models.Session

	// Scan session directories (organized by project hash)
	projectHashDirs, err := os.ReadDir(sessionsDir)
	if err != nil {
		return sessions, nil // Return empty if we can't read
	}

	for _, projectHashDir := range projectHashDirs {
		if !projectHashDir.IsDir() {
			continue
		}

		projectSessionsPath := filepath.Join(sessionsDir, projectHashDir.Name())
		sessionFiles, err := os.ReadDir(projectSessionsPath)
		if err != nil {
			continue
		}

		for _, sessionFile := range sessionFiles {
			if !strings.HasPrefix(sessionFile.Name(), "ses_") || !strings.HasSuffix(sessionFile.Name(), ".json") {
				continue
			}

			sessionPath := filepath.Join(projectSessionsPath, sessionFile.Name())
			data, err := os.ReadFile(sessionPath)
			if err != nil {
				continue
			}

			var session struct {
				ID        string `json:"id"`
				Version   string `json:"version"`
				ProjectID string `json:"projectID"`
				Directory string `json:"directory"`
				Title     string `json:"title"`
				Time      struct {
					Created int64 `json:"created"`
					Updated int64 `json:"updated"`
				} `json:"time"`
			}
			if err := json.Unmarshal(data, &session); err != nil {
				continue
			}

			// Determine the working directory
			workDir := session.Directory
			if workDir == "" {
				workDir = projectMap[session.ProjectID]
			}

			// Parse repo and branch from working directory
			repo := ""
			branch := ""
			if workDir != "" {
				pathParts := strings.Split(workDir, string(filepath.Separator))
				for i, part := range pathParts {
					if part == ".grove-worktrees" && i > 0 && i+1 < len(pathParts) {
						repo = pathParts[i-1]
						branch = pathParts[i+1]
						break
					}
				}
				if repo == "" {
					repo = filepath.Base(workDir)
				}
			}

			// Convert timestamp (milliseconds to time.Time)
			startedAt := time.Unix(0, session.Time.Created*int64(time.Millisecond))
			lastActivity := time.Unix(0, session.Time.Updated*int64(time.Millisecond))

			sessions = append(sessions, &models.Session{
				ID:               session.ID,
				Type:             "opencode_session",
				Status:           "completed", // OpenCode sessions are read-only for now
				Repo:             repo,
				Branch:           branch,
				WorkingDirectory: workDir,
				User:             os.Getenv("USER"),
				StartedAt:        startedAt,
				LastActivity:     lastActivity,
				JobTitle:         session.Title,
				Provider:         "opencode",
			})
		}
	}

	return sessions, nil
}
