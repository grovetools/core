package sanitize

import (
	"bytes"
	"testing"
)

func TestForDockerLabel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"simple string", "hello", "hello"},
		{"with spaces", "hello world", "hello_world"},
		{"with dots", "hello.world", "hello_world"},
		{"with hyphens", "hello-world", "hello_world"},
		{"mixed separators", "hello.world-foo bar", "hello_world_foo_bar"},
		{"special characters", "hello@world#foo", "hello_world_foo"},
		{"multiple underscores", "hello___world", "hello_world"},
		{"leading/trailing underscores", "_hello_world_", "hello_world"},
		{"uppercase", "HelloWorld", "helloworld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ForDockerLabel(tt.input)
			if result != tt.expected {
				t.Errorf("ForDockerLabel(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestForDomainPart(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"simple string", "hello", "hello"},
		{"with spaces", "hello world", "hello-world"},
		{"with dots", "hello.world", "hello-world"},
		{"with underscores", "hello_world", "hello-world"},
		{"special characters", "hello@world!", "hello-world"},
		{"multiple hyphens", "hello---world", "hello-world"},
		{"leading/trailing hyphens", "-hello-world-", "hello-world"},
		{"uppercase", "HelloWorld", "helloworld"},
		{"numbers", "hello123world", "hello123world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ForDomainPart(tt.input)
			if result != tt.expected {
				t.Errorf("ForDomainPart(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestForProjectName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"simple string", "myproject", "myproject"},
		{"with hyphens", "my-project", "my_project"},
		{"with dots", "my.project", "my_project"},
		{"starts with number", "123project", "grove_123project"},
		{"very long name", "this_is_a_very_long_project_name_that_exceeds_the_maximum_allowed_length_for_docker_compose", "this_is_a_very_long_project_name_that_exceeds_the_maximum_allow"},
		{"uppercase", "MyProject", "myproject"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ForProjectName(tt.input)
			if result != tt.expected {
				t.Errorf("ForProjectName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestForServiceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"simple string", "myservice", "myservice"},
		{"with spaces", "my service", "my-service"},
		{"with dots", "my.service", "my-service"},
		{"with underscores", "my_service", "my_service"},
		{"special characters", "my@service!", "myservice"},
		{"starts with special char", "@_service", "service-_service"},
		{"uppercase", "MyService", "MyService"},
		{"numbers", "service123", "service123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ForServiceName(tt.input)
			if result != tt.expected {
				t.Errorf("ForServiceName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestForEnvironmentKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"simple string", "mykey", "MYKEY"},
		{"with hyphens", "my-key", "MY_KEY"},
		{"with dots", "my.key", "MY_KEY"},
		{"with spaces", "my key", "MY_KEY"},
		{"special characters", "my@key!", "MY_KEY"},
		{"starts with number", "123key", "ENV_123KEY"},
		{"mixed case", "MyKey", "MYKEY"},
		{"multiple underscores", "my___key", "MY_KEY"},
		{"leading/trailing underscores", "_my_key_", "MY_KEY"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ForEnvironmentKey(tt.input)
			if result != tt.expected {
				t.Errorf("ForEnvironmentKey(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "valid UTF-8",
			input:    []byte("Hello, 世界! 🌍"),
			expected: "Hello, 世界! 🌍",
		},
		{
			name:     "invalid UTF-8 byte",
			input:    []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0xcf}, // "Hello " + invalid byte
			expected: "Hello �",
		},
		{
			name:     "mixed valid and invalid",
			input:    []byte("Valid text " + string([]byte{0xff, 0xfe}) + " more text"),
			expected: "Valid text �� more text",
		},
		{
			name:     "invalid continuation byte at position 447171",
			input:    append(bytes.Repeat([]byte("a"), 447171), 0xcf), // Simulating the exact error
			expected: string(bytes.Repeat([]byte("a"), 447171)) + "�",
		},
		{
			name:     "empty input",
			input:    []byte{},
			expected: "",
		},
		{
			name:     "all invalid bytes",
			input:    []byte{0xff, 0xfe, 0xfd},
			expected: "���",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UTF8(tt.input)
			if result != tt.expected {
				t.Errorf("UTF8() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func BenchmarkUTF8Valid(b *testing.B) {
	data := []byte("This is valid UTF-8 text with some unicode: 世界 🌍")
	for i := 0; i < b.N; i++ {
		_ = UTF8(data)
	}
}

func BenchmarkUTF8Invalid(b *testing.B) {
	data := append([]byte("This has invalid bytes: "), 0xff, 0xfe, 0xcf)
	for i := 0; i < b.N; i++ {
		_ = UTF8(data)
	}
}