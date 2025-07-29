package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateServiceName(t *testing.T) {
	testCases := []struct {
		name    string
		service string
		valid   bool
	}{
		{"valid simple", "api", true},
		{"valid with numbers", "api2", true},
		{"valid with dash", "my-api", true},
		{"valid with underscore", "my_api", true},
		{"invalid starts with number", "2api", false},
		{"invalid special char", "api@service", false},
		{"invalid space", "api service", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateServiceName(tc.service)
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	// Valid config
	valid := &Config{
		Version: "1.0",
		Services: map[string]ServiceConfig{
			"api": {
				Build: ".",
				Ports: []string{"8080"},
			},
		},
		Settings: Settings{
			NetworkName:  "grove",
			DomainSuffix: "localhost",
		},
	}

	assert.NoError(t, valid.Validate())

	// Missing version
	invalid := &Config{
		Services: map[string]ServiceConfig{
			"api": {Build: "."},
		},
	}
	assert.Error(t, invalid.Validate())

	// Service without build or image
	invalid = &Config{
		Version: "1.0",
		Services: map[string]ServiceConfig{
			"api": {},
		},
		Settings: Settings{
			NetworkName:  "grove",
			DomainSuffix: "localhost",
		},
	}
	assert.Error(t, invalid.Validate())

	// Invalid port format
	invalid = &Config{
		Version: "1.0",
		Services: map[string]ServiceConfig{
			"api": {
				Build: ".",
				Ports: []string{"not-a-port"},
			},
		},
		Settings: Settings{
			NetworkName:  "grove",
			DomainSuffix: "localhost",
		},
	}
	assert.Error(t, invalid.Validate())
}
