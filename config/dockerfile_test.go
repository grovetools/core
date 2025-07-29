package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDockerfileParser(t *testing.T) {
	// Create temporary Dockerfile
	tmpDir := t.TempDir()
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")

	content := `FROM node:14
WORKDIR /usr/src/app
COPY package*.json ./
RUN npm install
WORKDIR /app/server
COPY . .
CMD ["node", "index.js"]`

	if err := os.WriteFile(dockerfilePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	parser := NewDockerfileParser()
	workdir, err := parser.FindWorkdir(dockerfilePath)
	if err != nil {
		t.Fatal(err)
	}

	// Should return the last WORKDIR
	if workdir != "/app/server" {
		t.Errorf("expected /app/server, got %s", workdir)
	}
}

func TestDockerfileParserWithQuotes(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")

	content := `FROM node:14
WORKDIR "/app with spaces"
COPY . .`

	if err := os.WriteFile(dockerfilePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	parser := NewDockerfileParser()
	workdir, err := parser.FindWorkdir(dockerfilePath)
	if err != nil {
		t.Fatal(err)
	}

	if workdir != "/app with spaces" {
		t.Errorf("expected '/app with spaces', got %s", workdir)
	}
}

func TestDockerfileParserNoWorkdir(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")

	content := `FROM node:14
COPY . .
CMD ["node", "index.js"]`

	if err := os.WriteFile(dockerfilePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	parser := NewDockerfileParser()
	workdir, err := parser.FindWorkdir(dockerfilePath)
	if err != nil {
		t.Fatal(err)
	}

	// Should return empty string when no WORKDIR found
	if workdir != "" {
		t.Errorf("expected empty string, got %s", workdir)
	}
}

func TestFindDockerfile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a Dockerfile
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte("FROM node:14"), 0644); err != nil {
		t.Fatal(err)
	}

	parser := NewDockerfileParser()
	found, err := parser.FindDockerfile(tmpDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if found != dockerfilePath {
		t.Errorf("expected %s, got %s", dockerfilePath, found)
	}
}

func TestFindDockerfileCustom(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a custom Dockerfile
	customPath := filepath.Join(tmpDir, "Dockerfile.prod")
	if err := os.WriteFile(customPath, []byte("FROM node:14"), 0644); err != nil {
		t.Fatal(err)
	}

	parser := NewDockerfileParser()
	buildContext := map[string]interface{}{
		"dockerfile": "Dockerfile.prod",
	}

	found, err := parser.FindDockerfile(tmpDir, buildContext)
	if err != nil {
		t.Fatal(err)
	}

	if found != customPath {
		t.Errorf("expected %s, got %s", customPath, found)
	}
}

func TestFindDockerfileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	parser := NewDockerfileParser()
	_, err := parser.FindDockerfile(tmpDir, nil)
	if err == nil {
		t.Error("expected error when Dockerfile not found")
	}
}