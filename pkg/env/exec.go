package env

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// ExecProvider runs an external binary (e.g., grove-env-cloud) as an exec plugin.
// The binary receives an EnvRequest on stdin and writes an EnvResponse to stdout.
type ExecProvider struct {
	binaryName string
}

// NewExecProvider creates a provider that shells out to grove-env-<name>.
// If command is non-empty, it is used as the binary path directly;
// otherwise the binary is resolved from PATH as grove-env-<name>.
func NewExecProvider(name string, command string) *ExecProvider {
	bin := command
	if bin == "" {
		bin = fmt.Sprintf("grove-env-%s", name)
	}
	return &ExecProvider{
		binaryName: bin,
	}
}

func (p *ExecProvider) Up(ctx context.Context, req EnvRequest) (*EnvResponse, error) {
	req.Action = "up"
	return p.execute(ctx, req)
}

func (p *ExecProvider) Down(ctx context.Context, req EnvRequest) error {
	req.Action = "down"
	_, err := p.execute(ctx, req)
	return err
}

func (p *ExecProvider) execute(ctx context.Context, req EnvRequest) (*EnvResponse, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal env request: %w", err)
	}

	cmd := exec.CommandContext(ctx, p.binaryName)
	cmd.Stdin = bytes.NewReader(reqBytes)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("exec provider %s failed: %w\nStderr: %s", p.binaryName, err, stderr.String())
	}

	var resp EnvResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse provider response: %w\nStdout: %s", err, stdout.String())
	}

	if resp.Error != "" {
		return &resp, fmt.Errorf("provider returned error: %s", resp.Error)
	}

	return &resp, nil
}
