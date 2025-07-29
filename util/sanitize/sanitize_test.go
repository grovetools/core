package sanitize

import "testing"

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