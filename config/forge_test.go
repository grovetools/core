package config

import (
	"os"
	"strings"
	"testing"
)

func TestForgeAbsentIsNil(t *testing.T) {
	cfg, err := LoadFromTOMLBytes([]byte("name = \"demo\"\nversion = \"1.0\"\n"))
	if err != nil {
		t.Fatalf("LoadFromTOMLBytes failed: %v", err)
	}
	forge, err := cfg.Forge()
	if err != nil {
		t.Fatalf("Forge() failed: %v", err)
	}
	if forge != nil {
		t.Fatalf("Forge() = %+v for a config with no [forge] block, want nil", forge)
	}
}

func TestForgeParsesTOML(t *testing.T) {
	src := `
name = "demo"
version = "1.0"

[forge]
url = "https://forge.example.com"
remote_name = "myforge"
token_command = "op read op://grove/forge/token"
`
	cfg, err := LoadFromTOMLBytes([]byte(src))
	if err != nil {
		t.Fatalf("LoadFromTOMLBytes failed: %v", err)
	}
	forge, err := cfg.Forge()
	if err != nil {
		t.Fatalf("Forge() failed: %v", err)
	}
	if forge == nil {
		t.Fatal("Forge() = nil for a config with a [forge] block")
	}
	if forge.URL != "https://forge.example.com" {
		t.Errorf("URL = %q", forge.URL)
	}
	if forge.EffectiveRemoteName() != "myforge" {
		t.Errorf("EffectiveRemoteName = %q, want %q", forge.EffectiveRemoteName(), "myforge")
	}
	if forge.TokenCommand != "op read op://grove/forge/token" {
		t.Errorf("TokenCommand = %q", forge.TokenCommand)
	}
	if !forge.IsConfigured() {
		t.Error("IsConfigured = false for a block with a URL")
	}
}

func TestForgeRemoteNameDefaults(t *testing.T) {
	src := `
version = "1.0"

[forge]
url = "https://forge.example.com"
`
	cfg, err := LoadFromTOMLBytes([]byte(src))
	if err != nil {
		t.Fatalf("LoadFromTOMLBytes failed: %v", err)
	}
	forge, err := cfg.Forge()
	if err != nil {
		t.Fatalf("Forge() failed: %v", err)
	}
	if got := forge.EffectiveRemoteName(); got != DefaultForgeRemoteName {
		t.Errorf("EffectiveRemoteName = %q, want %q", got, DefaultForgeRemoteName)
	}
	// The default also holds for a nil block, so callers need no nil check.
	var nilCfg *ForgeConfig
	if got := nilCfg.EffectiveRemoteName(); got != DefaultForgeRemoteName {
		t.Errorf("nil EffectiveRemoteName = %q, want %q", got, DefaultForgeRemoteName)
	}
	if nilCfg.IsConfigured() {
		t.Error("a nil ForgeConfig reports IsConfigured")
	}
}

// TestForgeMalformedBlockDoesNotFailConfigLoad is the dark-build rule: a key
// nothing consumes yet must never be able to break config loading for
// everything else.
func TestForgeMalformedBlockDoesNotFailConfigLoad(t *testing.T) {
	src := `
name = "demo"
version = "1.0"

[forge]
url = "not a url at all"
remote_name = "-oProxyCommand=evil"
`
	cfg, err := LoadFromTOMLBytes([]byte(src))
	if err != nil {
		t.Fatalf("a malformed [forge] block broke config loading: %v", err)
	}
	if cfg.Name != "demo" {
		t.Errorf("Name = %q; the rest of the config did not survive", cfg.Name)
	}

	forge, err := cfg.Forge()
	if err != nil {
		t.Fatalf("Forge() failed: %v", err)
	}
	// Validate is explicit, and only then does the bad input surface.
	if err := forge.Validate(); err == nil {
		t.Error("Validate accepted a malformed [forge] block")
	}
}

func TestForgeValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *ForgeConfig
		wantErr string
	}{
		{"nil", nil, ""},
		{"empty", &ForgeConfig{}, ""},
		{"https", &ForgeConfig{URL: "https://forge.example.com"}, ""},
		{"http", &ForgeConfig{URL: "http://localhost:3000"}, ""},
		{"with path", &ForgeConfig{URL: "https://example.com/forge"}, ""},
		{"remote name", &ForgeConfig{URL: "https://f.example", RemoteName: "forge-2"}, ""},
		{"bad scheme", &ForgeConfig{URL: "ssh://forge.example.com"}, "must use http or https"},
		{"file scheme", &ForgeConfig{URL: "file:///etc/passwd"}, "must use http or https"},
		{"no host", &ForgeConfig{URL: "https://"}, "no host"},
		{"schemeless", &ForgeConfig{URL: "forge.example.com"}, "must use http or https"},
		{"dash remote", &ForgeConfig{RemoteName: "-x"}, "may not start with '-'"},
		{"space remote", &ForgeConfig{RemoteName: "my forge"}, "illegal character"},
		{"slash remote", &ForgeConfig{RemoteName: "a/b"}, "illegal character"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestForgeTokenCommandIsNeverExecuted guards the credential rule at the
// config layer: parsing a [forge] block must not run anything.
func TestForgeTokenCommandIsNeverExecuted(t *testing.T) {
	marker := t.TempDir() + "/token_command_was_executed"
	src := `
version = "1.0"

[forge]
url = "https://forge.example.com"
token_command = "touch ` + marker + `"
`
	cfg, err := LoadFromTOMLBytes([]byte(src))
	if err != nil {
		t.Fatalf("LoadFromTOMLBytes failed: %v", err)
	}
	forge, err := cfg.Forge()
	if err != nil {
		t.Fatalf("Forge() failed: %v", err)
	}
	if err := forge.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !strings.Contains(forge.TokenCommand, "touch") {
		t.Fatalf("TokenCommand was not carried through: %q", forge.TokenCommand)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("parsing or validating [forge] executed token_command")
	}
}

// TestForgeIsARegisteredExtension keeps `grove config audit` from reporting a
// legitimate [forge] block as an orphan key.
func TestForgeIsARegisteredExtension(t *testing.T) {
	info, ok := KnownExtension(ForgeExtensionKey)
	if !ok {
		t.Fatalf("%q is not in the extension registry", ForgeExtensionKey)
	}
	if info.Repo != "core" {
		t.Errorf("forge extension Repo = %q, want %q", info.Repo, "core")
	}
}
