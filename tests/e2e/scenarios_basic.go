package main

import (
	"github.com/grovetools/tend/pkg/assert"
	"github.com/grovetools/tend/pkg/command"
	"github.com/grovetools/tend/pkg/harness"
)

// VersionScenario tests the 'version' command.
func VersionScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "core-basic-version",
		Steps: []harness.Step{
			harness.NewStep("Run 'core version'", func(ctx *harness.Context) error {
				coreBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				cmd := command.New(coreBinary, "version")
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)

				if err := assert.Equal(0, result.ExitCode, "core version should exit successfully"); err != nil {
					return err
				}

				// Verify output contains version information
				if err := assert.Contains(result.Stdout, "Version:", "Output should contain Version"); err != nil {
					return err
				}
				if err := assert.Contains(result.Stdout, "Commit:", "Output should contain Commit"); err != nil {
					return err
				}
				if err := assert.Contains(result.Stdout, "Branch:", "Output should contain Branch"); err != nil {
					return err
				}
				return assert.Contains(result.Stdout, "Build Date:", "Output should contain Build Date")
			}),
		},
	}
}
