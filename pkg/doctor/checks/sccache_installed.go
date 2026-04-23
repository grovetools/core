package checks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/grovetools/core/pkg/doctor"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/pelletier/go-toml/v2"
)

func init() {
	doctor.Register(&sccacheInstalledCheck{
		getenv:       os.Getenv,
		getwd:        os.Getwd,
		resolveScope: workspace.ResolveScope,
		lookPath:     exec.LookPath,
		runVersion:   defaultSccacheVersion,
	})
}

type sccacheInstalledCheck struct {
	getenv       func(string) string
	getwd        func() (string, error)
	resolveScope func(dir string) string
	lookPath     func(name string) (string, error)
	runVersion   func(path string) (string, error)
}

func (c *sccacheInstalledCheck) ID() string   { return "sccache_installed" }
func (c *sccacheInstalledCheck) Name() string { return "sccache available for cargo-using services" }

func (c *sccacheInstalledCheck) Run(ctx context.Context, opts doctor.RunOptions) doctor.CheckResult {
	res := doctor.CheckResult{ID: c.ID(), Name: c.Name()}

	scope := c.currentScope()
	if scope == "" {
		res.Status = doctor.StatusOK
		res.Message = "no current scope (cwd/GROVE_SCOPE); skipping sccache check"
		return res
	}

	grovePath := filepath.Join(scope, "grove.toml")
	data, err := os.ReadFile(grovePath)
	if err != nil {
		res.Status = doctor.StatusOK
		res.Message = fmt.Sprintf("no grove.toml at %s; skipping sccache check", grovePath)
		return res
	}

	hasCargoService, flagged := scanCargoServices(data, scope)
	if !hasCargoService {
		res.Status = doctor.StatusOK
		res.Message = "no cargo-using services declared; sccache not required"
		return res
	}

	path, err := c.lookPath("sccache")
	if err != nil {
		res.Status = doctor.StatusWarn
		res.Message = fmt.Sprintf("sccache not found on PATH; cargo-using services: %s", strings.Join(flagged, ", "))
		res.Resolution = "Install sccache for cross-worktree compile cache: brew install sccache (or cargo install sccache)"
		return res
	}

	version, _ := c.runVersion(path)
	if version == "" {
		version = "unknown"
	}
	// `sccache --version` prints "sccache <ver>"; strip the prefix to avoid
	// "sccache sccache 0.14.0" in the message.
	version = strings.TrimPrefix(version, "sccache ")
	res.Status = doctor.StatusOK
	res.Message = fmt.Sprintf("sccache %s installed at %s", version, path)
	return res
}

func (c *sccacheInstalledCheck) AutoFix(ctx context.Context) error {
	return fmt.Errorf("%w: install sccache manually — brew install sccache (or cargo install sccache)", doctor.ErrNotFixable)
}

func (c *sccacheInstalledCheck) currentScope() string {
	if s := strings.TrimSpace(c.getenv("GROVE_SCOPE")); s != "" {
		return s
	}
	cwd, err := c.getwd()
	if err != nil {
		return ""
	}
	return c.resolveScope(cwd)
}

var cargoWordRe = regexp.MustCompile(`\bcargo\b`)

// scanCargoServices parses grove.toml data and returns whether any service
// uses cargo and a list of the flagged service names. A service is "cargo-using"
// if its `command` field contains `cargo` as a whole word, OR if a .cargo/config.toml
// file exists under scope (indicating the workspace is a Cargo project).
func scanCargoServices(data []byte, scope string) (bool, []string) {
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return false, nil
	}

	var flagged []string
	seen := map[string]bool{}

	walkServices(root, func(envName, svcName string, svc map[string]any) {
		cmd, _ := svc["command"].(string)
		if cmd != "" && cargoWordRe.MatchString(cmd) {
			key := envName + "." + svcName
			if !seen[key] {
				seen[key] = true
				flagged = append(flagged, key)
			}
			return
		}
		if lc, ok := svc["lifecycle"].(map[string]any); ok {
			ps, _ := lc["post_start"].(string)
			if ps != "" && cargoWordRe.MatchString(ps) {
				key := envName + "." + svcName + "(post_start)"
				if !seen[key] {
					seen[key] = true
					flagged = append(flagged, key)
				}
			}
		}
	})

	if len(flagged) > 0 {
		return true, flagged
	}

	// Fallback: .cargo/config.toml anywhere under scope (shallow walk).
	if findCargoConfig(scope) {
		return true, []string{".cargo/config.toml"}
	}
	return false, nil
}

func walkServices(root map[string]any, fn func(envName, svcName string, svc map[string]any)) {
	// Default environment: [environment.config.services.<name>]
	if env, ok := root["environment"].(map[string]any); ok {
		if cfg, ok := env["config"].(map[string]any); ok {
			if svcs, ok := cfg["services"].(map[string]any); ok {
				for name, raw := range svcs {
					if svc, ok := raw.(map[string]any); ok {
						fn("environment", name, svc)
					}
				}
			}
		}
	}
	// Named environments: [environments.<env>.config.services.<name>]
	if envs, ok := root["environments"].(map[string]any); ok {
		for envName, envRaw := range envs {
			env, ok := envRaw.(map[string]any)
			if !ok {
				continue
			}
			cfg, ok := env["config"].(map[string]any)
			if !ok {
				continue
			}
			svcs, ok := cfg["services"].(map[string]any)
			if !ok {
				continue
			}
			for name, raw := range svcs {
				if svc, ok := raw.(map[string]any); ok {
					fn(envName, name, svc)
				}
			}
		}
	}
}

func findCargoConfig(scope string) bool {
	// Look two levels deep: scope/.cargo/config.toml and scope/<ws>/.cargo/config.toml.
	if _, err := os.Stat(filepath.Join(scope, ".cargo", "config.toml")); err == nil {
		return true
	}
	entries, err := os.ReadDir(scope)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if _, err := os.Stat(filepath.Join(scope, e.Name(), ".cargo", "config.toml")); err == nil {
			return true
		}
	}
	return false
}

func defaultSccacheVersion(path string) (string, error) {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
