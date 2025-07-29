package command

import (
	"context"
	"testing"
	"time"
)

func TestValidateProjectName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid name", "my-project", false},
		{"valid with underscore", "my_project", false},
		{"valid with numbers", "project123", false},
		{"empty name", "", true},
		{"uppercase letters", "MyProject", true},
		{"special characters", "my@project", true},
		{"starts with hyphen", "-project", true},
		{"too long", "this-is-a-very-long-project-name-that-exceeds-the-maximum-allowed-length", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProjectName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProjectName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateServiceName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid name", "my-service", false},
		{"valid with underscore", "my_service", false},
		{"valid with uppercase", "MyService", false},
		{"valid with numbers", "service123", false},
		{"empty name", "", true},
		{"special characters", "my@service", true},
		{"starts with hyphen", "-service", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServiceName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateServiceName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateFileName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid path", "/path/to/file.txt", false},
		{"relative path", "relative/path.txt", false},
		{"directory traversal", "etc/passwd", true},
		{"command injection semicolon", "file.txt; rm -rf /", true},
		{"command injection pipe", "file.txt | cat", true},
		{"command injection ampersand", "file.txt & echo", true},
		{"command injection dollar", "$(whoami)", true},
		{"command injection backtick", "`whoami`", true},
		{"empty path", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFileName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFileName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateGitRef(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid branch", "main", false},
		{"valid with slash", "feature/add-button", false},
		{"valid with hyphen", "fix-bug", false},
		{"valid with underscore", "my_branch", false},
		{"valid with dots", "v1.2.3", false},
		{"valid tag", "v1.0.0", false},
		{"empty ref", "", true},
		{"command injection", "main; rm -rf /", true},
		{"spaces", "my branch", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitRef(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateGitRef(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestSafeBuilder_Build(t *testing.T) {
	sb := NewSafeBuilder()
	ctx := context.Background()

	t.Run("valid command", func(t *testing.T) {
		cmd, err := sb.Build(ctx, "echo", "hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmd.name != "echo" {
			t.Errorf("expected command name 'echo', got %q", cmd.name)
		}
		if len(cmd.args) != 1 || cmd.args[0] != "hello" {
			t.Errorf("expected args ['hello'], got %v", cmd.args)
		}
	})

	t.Run("empty command name", func(t *testing.T) {
		_, err := sb.Build(ctx, "")
		if err == nil {
			t.Error("expected error for empty command name")
		}
	})
}

func TestSafeBuilder_Validate(t *testing.T) {
	sb := NewSafeBuilder()

	t.Run("valid project name", func(t *testing.T) {
		err := sb.Validate("projectName", "my-project")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid project name", func(t *testing.T) {
		err := sb.Validate("projectName", "My-Project")
		if err == nil {
			t.Error("expected error for invalid project name")
		}
	})

	t.Run("unknown validator type", func(t *testing.T) {
		err := sb.Validate("unknownType", "value")
		if err == nil {
			t.Error("expected error for unknown validator type")
		}
	})
}

func TestCommand_WithTimeout(t *testing.T) {
	sb := NewSafeBuilder()
	ctx := context.Background()

	cmd, err := sb.Build(ctx, "sleep", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("custom timeout", func(t *testing.T) {
		customTimeout := 1 * time.Second
		cmd = cmd.WithTimeout(customTimeout)
		if cmd.timeout != customTimeout {
			t.Errorf("expected timeout %v, got %v", customTimeout, cmd.timeout)
		}
	})

	t.Run("exceeds max timeout", func(t *testing.T) {
		cmd = cmd.WithTimeout(20 * time.Minute)
		if cmd.timeout != MaxTimeout {
			t.Errorf("expected timeout to be capped at %v, got %v", MaxTimeout, cmd.timeout)
		}
	})
}

func TestCommandTimeout(t *testing.T) {
	sb := NewSafeBuilder()
	ctx := context.Background()

	// Create a command that will timeout
	cmd, err := sb.Build(ctx, "sleep", "10")
	if err != nil {
		t.Fatal(err)
	}

	// Set a short timeout
	cmd = cmd.WithTimeout(100 * time.Millisecond)

	start := time.Now()
	err = cmd.Exec().Run()
	duration := time.Since(start)

	if err == nil {
		t.Error("expected timeout error")
	}

	// Allow some margin for execution overhead
	if duration > 500*time.Millisecond {
		t.Errorf("command took too long to timeout: %v", duration)
	}
}