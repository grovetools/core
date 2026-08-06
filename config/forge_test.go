package config

import (
	"os"
	"strings"
	"testing"
	"time"
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

// TestForgePollOffByDefault is the wave's default-OFF gate: a [forge] block
// with no poll sub-block, and no [forge] block at all, must both leave the
// daemon poller dark.
func TestForgePollOffByDefault(t *testing.T) {
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
	if forge.Poll != nil {
		t.Fatalf("Poll = %+v for a block with no [forge.poll], want nil", forge.Poll)
	}
	if forge.PollEnabled() {
		t.Error("PollEnabled = true for a [forge] block with no poll sub-block")
	}

	var absent *ForgeConfig
	if absent.PollEnabled() {
		t.Error("PollEnabled = true for a nil ForgeConfig")
	}

	explicitlyOff := &ForgeConfig{Poll: &ForgePollConfig{Enabled: false}}
	if explicitlyOff.PollEnabled() {
		t.Error("PollEnabled = true for enabled = false")
	}
}

func TestForgePollParsesTOML(t *testing.T) {
	src := `
version = "1.0"

[forge.poll]
enabled = true
interval = "90s"
stale_after = "30m"
`
	cfg, err := LoadFromTOMLBytes([]byte(src))
	if err != nil {
		t.Fatalf("LoadFromTOMLBytes failed: %v", err)
	}
	forge, err := cfg.Forge()
	if err != nil {
		t.Fatalf("Forge() failed: %v", err)
	}
	if forge == nil || forge.Poll == nil {
		t.Fatalf("Forge()/Poll = %+v, want a parsed poll block", forge)
	}
	if !forge.PollEnabled() {
		t.Error("PollEnabled = false for enabled = true")
	}
	if got := forge.Poll.EffectiveInterval(); got != 90*time.Second {
		t.Errorf("EffectiveInterval = %v, want 90s", got)
	}
	if got := forge.Poll.EffectiveStaleAfter(); got != 30*time.Minute {
		t.Errorf("EffectiveStaleAfter = %v, want 30m", got)
	}
	// A poll block does not make a self-hosted forge configured — the two are
	// independent, which is what lets the poller run against GitHub with no
	// [forge] url at all.
	if forge.IsConfigured() {
		t.Error("IsConfigured = true for a block carrying only [forge.poll]")
	}
}

func TestForgeInfraCutoverDefaultsAndValidation(t *testing.T) {
	var absent *ForgeInfraConfig
	if !absent.SyncdIngressIsEnabled() || !absent.ForgejoIngressIsEnabled() {
		t.Fatal("absent ingress switches must preserve both service rules")
	}
	if got := absent.EffectiveSSHIngress(); got != ForgeSSHIngressCIDRAndIAP {
		t.Fatalf("absent ssh_ingress = %q, want %q", got, ForgeSSHIngressCIDRAndIAP)
	}

	off := false
	cutover := &ForgeInfraConfig{
		SyncdIngressEnabled:   &off,
		ForgejoIngressEnabled: &off,
		SSHIngress:            ForgeSSHIngressIAP,
	}
	if cutover.SyncdIngressIsEnabled() || cutover.ForgejoIngressIsEnabled() {
		t.Fatal("explicit false ingress switches were ignored")
	}
	if err := cutover.Validate(); err != nil {
		t.Fatalf("full cutover shape rejected: %v", err)
	}

	for _, valid := range []string{"", ForgeSSHIngressCIDRAndIAP, ForgeSSHIngressIAP, ForgeSSHIngressCIDR, " IAP "} {
		if err := (&ForgeInfraConfig{SSHIngress: valid}).Validate(); err != nil {
			t.Errorf("ssh_ingress %q rejected: %v", valid, err)
		}
	}
	if err := (&ForgeInfraConfig{SSHIngress: "public"}).Validate(); err == nil || !strings.Contains(err.Error(), "ssh_ingress") {
		t.Fatalf("invalid ssh_ingress error = %v", err)
	}
}

func TestForgeInfraCutoverFieldsParseWithoutLoadValidation(t *testing.T) {
	cfg, err := LoadFromTOMLBytes([]byte(`version = "1.0"
[forge.infra]
syncd_ingress_enabled = false
forgejo_ingress_enabled = false
ssh_ingress = "invalid-until-used"
`))
	if err != nil {
		t.Fatalf("load must remain parse-only: %v", err)
	}
	forge, err := cfg.Forge()
	if err != nil {
		t.Fatal(err)
	}
	if forge == nil || forge.Infra == nil || forge.Infra.SyncdIngressIsEnabled() || forge.Infra.ForgejoIngressIsEnabled() {
		t.Fatalf("parsed forge infra = %#v", forge)
	}
	if err := forge.Validate(); err == nil || !strings.Contains(err.Error(), "ssh_ingress") {
		t.Fatalf("use-time validation error = %v", err)
	}
}

func TestForgePollCadenceBounds(t *testing.T) {
	cases := []struct {
		name           string
		poll           *ForgePollConfig
		wantInterval   time.Duration
		wantStaleAfter time.Duration
	}{
		{
			name:           "empty falls back to defaults",
			poll:           &ForgePollConfig{Enabled: true},
			wantInterval:   DefaultForgePollInterval,
			wantStaleAfter: DefaultForgeStaleAfter,
		},
		{
			name:           "sub-floor interval is clamped up",
			poll:           &ForgePollConfig{Enabled: true, Interval: "1s"},
			wantInterval:   MinForgePollInterval,
			wantStaleAfter: DefaultForgeStaleAfter,
		},
		{
			name:           "unparseable durations fall back rather than fail",
			poll:           &ForgePollConfig{Enabled: true, Interval: "every so often", StaleAfter: "?"},
			wantInterval:   DefaultForgePollInterval,
			wantStaleAfter: DefaultForgeStaleAfter,
		},
		{
			name:           "stale window never narrower than the sweep gap",
			poll:           &ForgePollConfig{Enabled: true, Interval: "30m", StaleAfter: "5m"},
			wantInterval:   30 * time.Minute,
			wantStaleAfter: 30 * time.Minute,
		},
		{
			name:           "nil poll config is still answerable",
			poll:           nil,
			wantInterval:   DefaultForgePollInterval,
			wantStaleAfter: DefaultForgeStaleAfter,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.poll.EffectiveInterval(); got != tc.wantInterval {
				t.Errorf("EffectiveInterval = %v, want %v", got, tc.wantInterval)
			}
			if got := tc.poll.EffectiveStaleAfter(); got != tc.wantStaleAfter {
				t.Errorf("EffectiveStaleAfter = %v, want %v", got, tc.wantStaleAfter)
			}
		})
	}
}

// TestForgePollValidateReportsBadDuration pins the pairing: Validate tells a
// human about a typo, while the runtime accessors above silently fall back.
// A daemon must not fail to boot over a poll cadence.
func TestForgePollValidateReportsBadDuration(t *testing.T) {
	bad := &ForgeConfig{Poll: &ForgePollConfig{Enabled: true, Interval: "5 minutes"}}
	err := bad.Validate()
	if err == nil {
		t.Fatal("Validate() = nil for an unparseable interval")
	}
	if !strings.Contains(err.Error(), "interval") {
		t.Errorf("Validate() error = %q, want it to name the field", err)
	}
	if got := bad.Poll.EffectiveInterval(); got != DefaultForgePollInterval {
		t.Errorf("EffectiveInterval = %v after a bad value, want the default %v", got, DefaultForgePollInterval)
	}

	negative := &ForgeConfig{Poll: &ForgePollConfig{Enabled: true, StaleAfter: "-5m"}}
	if err := negative.Validate(); err == nil {
		t.Fatal("Validate() = nil for a negative stale_after")
	}

	ok := &ForgeConfig{Poll: &ForgePollConfig{Enabled: true, Interval: "5m", StaleAfter: "1h"}}
	if err := ok.Validate(); err != nil {
		t.Errorf("Validate() = %v for a well-formed poll block", err)
	}
}
