package pitrust

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupHome points HOME at a temp dir and returns the trust.json path.
// mkAgentDir controls whether ~/.pi/agent exists (pi "installed").
func setupHome(t *testing.T, mkAgentDir bool) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	agentDir := filepath.Join(home, ".pi", "agent")
	if mkAgentDir {
		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			t.Fatalf("mkdir agent dir: %v", err)
		}
	}
	return filepath.Join(agentDir, "trust.json")
}

func readTrust(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trust file: %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("trust.json should end with a newline (pi writeTrustFile format)")
	}
	m := map[string]any{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse trust file: %v", err)
	}
	return m
}

func TestSeedTrust_CreatesFreshFile(t *testing.T) {
	trustPath := setupHome(t, true)

	if err := SeedTrust("/tmp/wt-container"); err != nil {
		t.Fatalf("SeedTrust: %v", err)
	}

	m := readTrust(t, trustPath)
	if m["/tmp/wt-container"] != true {
		t.Errorf("expected /tmp/wt-container trusted, got %v", m["/tmp/wt-container"])
	}
}

func TestSeedTrust_NoOpWhenPiNotInstalled(t *testing.T) {
	trustPath := setupHome(t, false)

	if err := SeedTrust("/tmp/wt"); err != nil {
		t.Fatalf("SeedTrust should no-op without ~/.pi/agent, got: %v", err)
	}
	if _, err := os.Stat(trustPath); !os.IsNotExist(err) {
		t.Error("trust.json must not be created when ~/.pi/agent does not exist")
	}
}

func TestSeedTrust_PreservesExistingEntries(t *testing.T) {
	trustPath := setupHome(t, true)
	existing := "{\n  \"/home/u/denied\": false,\n  \"/home/u/other\": true\n}\n"
	if err := os.WriteFile(trustPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SeedTrust("/tmp/wt"); err != nil {
		t.Fatalf("SeedTrust: %v", err)
	}

	m := readTrust(t, trustPath)
	if m["/home/u/denied"] != false {
		t.Errorf("explicit denial must survive, got %v", m["/home/u/denied"])
	}
	if m["/home/u/other"] != true || m["/tmp/wt"] != true {
		t.Errorf("entries wrong: %v", m)
	}
}

func TestSeedTrust_OverridesDenialForSeededPath(t *testing.T) {
	trustPath := setupHome(t, true)
	if err := os.WriteFile(trustPath, []byte(`{"/tmp/wt": false}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SeedTrust("/tmp/wt"); err != nil {
		t.Fatalf("SeedTrust: %v", err)
	}
	if m := readTrust(t, trustPath); m["/tmp/wt"] != true {
		t.Errorf("seeded path should be forced true, got %v", m["/tmp/wt"])
	}
}

func TestSeedTrust_MalformedFileUntouched(t *testing.T) {
	trustPath := setupHome(t, true)
	if err := os.WriteFile(trustPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := SeedTrust("/tmp/wt")
	if err == nil {
		t.Fatal("expected error for malformed trust.json")
	}
	data, rerr := os.ReadFile(trustPath)
	if rerr != nil || string(data) != "{not json" {
		t.Errorf("malformed file must be left byte-for-byte untouched, got %q (%v)", data, rerr)
	}
}

func TestSeedTrust_EnvGateOff(t *testing.T) {
	trustPath := setupHome(t, true)
	t.Setenv("GROVE_PRESEED_PI_TRUST", "off")

	if err := SeedTrust("/tmp/wt"); err != nil {
		t.Fatalf("SeedTrust: %v", err)
	}
	if _, err := os.Stat(trustPath); !os.IsNotExist(err) {
		t.Error("gate off must leave trust.json absent")
	}
}

func TestSeedTrust_NoPathsNoOp(t *testing.T) {
	trustPath := setupHome(t, true)
	if err := SeedTrust(); err != nil {
		t.Fatalf("SeedTrust: %v", err)
	}
	if _, err := os.Stat(trustPath); !os.IsNotExist(err) {
		t.Error("no paths must not create trust.json")
	}
}

func TestSeedTrustForConfigDir_SeparatesStockAndGroveAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{".pi", ".grove-agent"} {
		if err := os.MkdirAll(filepath.Join(home, dir, "agent"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := SeedTrust("/stock"); err != nil {
		t.Fatal(err)
	}
	if err := SeedTrustForConfigDir(".grove-agent", "/product"); err != nil {
		t.Fatal(err)
	}
	stock := readTrust(t, filepath.Join(home, ".pi", "agent", "trust.json"))
	product := readTrust(t, filepath.Join(home, ".grove-agent", "agent", "trust.json"))
	if stock["/stock"] != true || stock["/product"] != nil {
		t.Fatalf("stock trust crossed profiles: %#v", stock)
	}
	if product["/product"] != true || product["/stock"] != nil {
		t.Fatalf("grove-agent trust crossed profiles: %#v", product)
	}
	if err := SeedTrustForConfigDir("../escape", "/x"); err == nil {
		t.Fatal("expected invalid config directory rejection")
	}
}

func TestSeedTrust_SortedKeys(t *testing.T) {
	trustPath := setupHome(t, true)
	if err := SeedTrust("/z/last", "/a/first"); err != nil {
		t.Fatalf("SeedTrust: %v", err)
	}
	data, err := os.ReadFile(trustPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(string(data), "/a/first") > strings.Index(string(data), "/z/last") {
		t.Errorf("keys should be sorted (pi writeTrustFile format):\n%s", data)
	}
}
