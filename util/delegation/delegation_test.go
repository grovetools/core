package delegation

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCommand(t *testing.T) {
	tests := []struct {
		name         string
		tool         string
		args         []string
		groveInPath  bool
		expectedPath string
		expectedArgs []string
	}{
		{
			name:         "with grove in PATH",
			tool:         "flow",
			args:         []string{"plan", "status"},
			groveInPath:  true,
			expectedPath: "grove",
			expectedArgs: []string{"grove", "flow", "plan", "status"},
		},
		{
			name:         "without grove in PATH",
			tool:         "flow",
			args:         []string{"plan", "status"},
			groveInPath:  false,
			expectedPath: "flow",
			expectedArgs: []string{"flow", "plan", "status"},
		},
		{
			name:         "single arg with grove",
			tool:         "cx",
			args:         []string{"rules"},
			groveInPath:  true,
			expectedPath: "grove",
			expectedArgs: []string{"grove", "cx", "rules"},
		},
		{
			name:         "no args with grove",
			tool:         "nb",
			args:         []string{},
			groveInPath:  true,
			expectedPath: "grove",
			expectedArgs: []string{"grove", "nb"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			if tt.groveInPath {
				// Create a temporary directory and add a fake grove binary
				tmpDir := t.TempDir()
				grovePath := filepath.Join(tmpDir, "grove")
				if err := os.WriteFile(grovePath, []byte("#!/bin/sh\n"), 0755); err != nil {
					t.Fatalf("failed to create fake grove binary: %v", err)
				}

				// Modify PATH to include our temp directory
				oldPath := os.Getenv("PATH")
				os.Setenv("PATH", tmpDir+":"+oldPath)
				defer os.Setenv("PATH", oldPath)
			} else {
				// Ensure grove is NOT in PATH
				// We'll set PATH to a minimal value that doesn't include grove
				oldPath := os.Getenv("PATH")
				os.Setenv("PATH", "/usr/bin:/bin")
				defer os.Setenv("PATH", oldPath)
			}

			// Execute the function
			cmd := Command(tt.tool, tt.args...)

			// Verify the command path
			if cmd.Path != tt.expectedPath && filepath.Base(cmd.Path) != tt.expectedPath {
				// LookPath may return full path, so check both
				t.Errorf("expected path %q, got %q", tt.expectedPath, cmd.Path)
			}

			// Verify the arguments
			if len(cmd.Args) != len(tt.expectedArgs) {
				t.Errorf("expected %d args, got %d: %v", len(tt.expectedArgs), len(cmd.Args), cmd.Args)
			} else {
				for i, arg := range tt.expectedArgs {
					if cmd.Args[i] != arg {
						t.Errorf("arg[%d]: expected %q, got %q", i, arg, cmd.Args[i])
					}
				}
			}
		})
	}
}

func TestCommandIntegration(t *testing.T) {
	// Test that the command can actually be constructed without errors
	cmd := Command("echo", "hello", "world")
	if cmd == nil {
		t.Fatal("Command returned nil")
	}

	// Verify it's an exec.Cmd
	if _, ok := interface{}(cmd).(*exec.Cmd); !ok {
		t.Error("Command did not return *exec.Cmd")
	}
}
