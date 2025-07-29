package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/command"
	"github.com/mattsolo1/grove-core/util/sanitize"
)

// InferDefaults applies smart defaults based on project structure
func (c *Config) InferDefaults() {
	// Infer network name from repo + branch if not specified
	if c.Settings.NetworkName == "grove" {
		// Try to get git info for a better network name
		if repo, branch, err := getGitInfo("."); err == nil {
			c.Settings.NetworkName = generateProjectName(repo, branch)
		}
	}

	// Create default profile if none exist
	if len(c.Profiles) == 0 && len(c.Services) > 0 {
		serviceNames := make([]string, 0, len(c.Services))
		for name := range c.Services {
			serviceNames = append(serviceNames, name)
		}

		c.Profiles = map[string]ProfileConfig{
			"default": {Services: serviceNames},
		}
	}

	// Set default profile if not specified
	if c.Settings.DefaultProfile == "" && len(c.Profiles) > 0 {
		// If there's a profile named "default", use it
		if _, hasDefault := c.Profiles["default"]; hasDefault {
			c.Settings.DefaultProfile = "default"
		} else {
			// Otherwise use the first profile
			for name := range c.Profiles {
				c.Settings.DefaultProfile = name
				break
			}
		}
	}

	// Infer service-specific defaults
	for name, service := range c.Services {
		inferredService := service
		inferServiceDefaults(name, &inferredService)
		c.Services[name] = inferredService
	}
}

// inferServiceDefaults applies smart defaults to a service based on common patterns
func inferServiceDefaults(serviceName string, service *ServiceConfig) {
	// Skip if both build and image are already specified
	if service.Build != nil || service.Image != "" {
		// If it's a Node.js project with build context, infer common settings
		if service.Build != nil && isNodeProject() {
			inferNodeDefaults(service)
		}
		return
	}

	// If neither build nor image is specified, check for common patterns
	if isNodeProject() {
		service.Build = "."
		inferNodeDefaults(service)
	}
}

// isNodeProject checks if the current directory is a Node.js project
func isNodeProject() bool {
	_, err := os.Stat("package.json")
	return err == nil
}

// inferNodeDefaults applies Node.js-specific defaults
func inferNodeDefaults(service *ServiceConfig) {
	// Add NODE_ENV if not already set
	hasNodeEnv := false
	for _, env := range service.Environment {
		if strings.HasPrefix(env, "NODE_ENV=") {
			hasNodeEnv = true
			break
		}
	}
	if !hasNodeEnv {
		service.Environment = append(service.Environment, "NODE_ENV=development")
	}

	// Infer common volumes if none are specified
	if len(service.Volumes) == 0 {
		workdir := detectWorkdir(service)
		service.Volumes = inferNodeVolumes(workdir)
	}
}

// detectWorkdir detects the working directory from Dockerfile or defaults to /app
func detectWorkdir(service *ServiceConfig) string {
	workdir := "/app" // default fallback

	// Only try to detect if there's a build context
	if service.Build == "" {
		return workdir
	}

	parser := NewDockerfileParser()

	// Convert build to map if it's a string
	var buildContext map[string]interface{}
	if service.Build == "." {
		buildContext = nil
	}

	// Try to find and parse Dockerfile
	dockerfilePath, err := parser.FindDockerfile(".", buildContext)
	if err == nil {
		if detectedWorkdir, err := parser.FindWorkdir(dockerfilePath); err == nil && detectedWorkdir != "" {
			workdir = detectedWorkdir
		}
	}

	return workdir
}

// inferNodeVolumes returns common volume mappings for Node.js projects
func inferNodeVolumes(workdir string) []string {
	var volumes []string

	// Common patterns for Node.js development
	patterns := map[string]string{
		"src":               fmt.Sprintf("./src:%s/src", workdir),
		"public":            fmt.Sprintf("./public:%s/public", workdir),
		"package.json":      fmt.Sprintf("./package.json:%s/package.json", workdir),
		"package-lock.json": fmt.Sprintf("./package-lock.json:%s/package-lock.json", workdir),
		"yarn.lock":         fmt.Sprintf("./yarn.lock:%s/yarn.lock", workdir),
		"pnpm-lock.yaml":    fmt.Sprintf("./pnpm-lock.yaml:%s/pnpm-lock.yaml", workdir),
		"tsconfig.json":     fmt.Sprintf("./tsconfig.json:%s/tsconfig.json", workdir),
		"vite.config.js":    fmt.Sprintf("./vite.config.js:%s/vite.config.js", workdir),
		"vite.config.ts":    fmt.Sprintf("./vite.config.ts:%s/vite.config.ts", workdir),
		"webpack.config.js": fmt.Sprintf("./webpack.config.js:%s/webpack.config.js", workdir),
		"next.config.js":    fmt.Sprintf("./next.config.js:%s/next.config.js", workdir),
		"index.html":        fmt.Sprintf("./index.html:%s/index.html", workdir),
		".env":              fmt.Sprintf("./.env:%s/.env", workdir),
		".env.local":        fmt.Sprintf("./.env.local:%s/.env.local", workdir),
	}

	for file, mapping := range patterns {
		if _, err := os.Stat(file); err == nil {
			volumes = append(volumes, mapping)
		}
	}

	// Check for directories
	dirPatterns := map[string]string{
		"src":        fmt.Sprintf("./src:%s/src", workdir),
		"public":     fmt.Sprintf("./public:%s/public", workdir),
		"static":     fmt.Sprintf("./static:%s/static", workdir),
		"assets":     fmt.Sprintf("./assets:%s/assets", workdir),
		"components": fmt.Sprintf("./components:%s/components", workdir),
		"pages":      fmt.Sprintf("./pages:%s/pages", workdir),
		"app":        fmt.Sprintf("./app:%s/app", workdir),
		"lib":        fmt.Sprintf("./lib:%s/lib", workdir),
		"utils":      fmt.Sprintf("./utils:%s/utils", workdir),
		"styles":     fmt.Sprintf("./styles:%s/styles", workdir),
		"config":     fmt.Sprintf("./config:%s/config", workdir),
	}

	for dir, mapping := range dirPatterns {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			// Check if not already added
			alreadyAdded := false
			for _, v := range volumes {
				if v == mapping {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded {
				volumes = append(volumes, mapping)
			}
		}
	}

	return volumes
}

// getGitInfo returns the repository name and current branch
func getGitInfo(path string) (repo string, branch string, err error) {
	cmdBuilder := command.NewSafeBuilder()

	// Get repository name
	cmd, err := cmdBuilder.Build(context.Background(), "git", "-C", path, "remote", "get-url", "origin")
	if err != nil {
		return "", "", fmt.Errorf("failed to build command: %w", err)
	}
	output, err := cmd.Exec().Output()
	if err != nil {
		// Fallback to directory name
		abs, _ := filepath.Abs(path)
		repo = filepath.Base(abs)
	} else {
		// Extract repo name from URL
		url := strings.TrimSpace(string(output))
		repo = extractRepoName(url)
	}

	// Get current branch
	cmd, err = cmdBuilder.Build(context.Background(), "git", "-C", path, "branch", "--show-current")
	if err != nil {
		return "", "", fmt.Errorf("failed to build command: %w", err)
	}
	output, err = cmd.Exec().Output()
	if err != nil {
		branch = "main"
	} else {
		branch = strings.TrimSpace(string(output))
		if branch == "" {
			branch = "main"
		}
	}

	return repo, branch, nil
}

// extractRepoName extracts repository name from git URL
func extractRepoName(url string) string {
	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// Handle different URL formats
	if strings.Contains(url, "") {
		// HTTPS URLs: https://github.com/user/repo
		parts := strings.Split(url, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	} else if strings.Contains(url, "@") {
		// SSH URLs: git@github.com:user/repo
		parts := strings.Split(url, ":")
		if len(parts) > 1 {
			pathParts := strings.Split(parts[1], "/")
			if len(pathParts) > 0 {
				return pathParts[len(pathParts)-1]
			}
		}
	}

	// Fallback
	return "grove"
}

// generateProjectName creates a sanitized project name from repo and branch
func generateProjectName(repo, branch string) string {
	name := fmt.Sprintf("%s-%s", repo, branch)
	return sanitize.ForDomainPart(name)
}
