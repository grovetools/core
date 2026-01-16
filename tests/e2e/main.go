package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/grovetools/tend/pkg/app"
	"github.com/grovetools/tend/pkg/harness"
)

func main() {
	// A list of all E2E scenarios for grove-core.
	scenarios := []*harness.Scenario{
		CoreConfigLayeringScenario(),
		CoreConfigOverrideScenario(),
		CoreConfigMissingScenario(),
		CoreConfigVersionScenario(),
		CoreConfigGlobalOverrideScenario(),
		CoreConfigGlobalOverridePrecedenceScenario(),
		CoreConfigGlobalOverrideYamlExtensionScenario(),
		WorkspaceModelScenario(),
		WorkspaceEdgeCasesScenario(),
		WorkspaceAdvancedScenario(),
		NotebooksConfigIsolationScenario(),
		NotebooksLocalModeScenario(),
		NotebooksXDGConfigHomeScenario(),
		// Logging scenarios
		WorkspaceLogCreationScenario(),
		WorkspaceRootLogPlacementScenario(),
		EcosystemLogPlacementScenario(),
		LoggingJSONFormatScenario(),
		LoggingJSONFieldsScenario(),
		LoggingNestedJSONScenario(),
		LoggingLevelFilterScenario(),
		JSONTreeComponentScenario(),
		// Component filtering scenarios
		LoggingComponentFilterDefaultScenario(),
		LoggingComponentFilterShowScenario(),
		LoggingComponentFilterHideScenario(),
		LoggingComponentFilterConsistencyScenario(),
		// CLI filtering flags scenario
		LogsCLIFilteringScenario(),
		// TUI scenarios
		LoggingTUITestScenario(),
		LoggingTUIVimNavigationScenario(),
		LoggingTUIJsonSearchScenario(),
		LoggingTUIVisualModeYankScenario(),
		LoggingTUIExistingLogsScenario(),
		LoggingTUINewFilesScenario(),
		LoggingTUIFilteringTestScenario(),
		// TUI sorting scenarios
		LoggingTUIChronologicalSortingScenario(),
		LoggingTUILiveUpdateSortingScenario(),
		LoggingTUIFollowModeSortingScenario(),
		// Zombie worktree scenarios
		ZombieWorktreeLogRecreationScenario(),
		// Nvim component scenarios
		// NvimDemoScenario(),
		// NvimDemoFileBrowserScenario(),
		// NvimDemoFileOpenScenario(),
	}

	// Setup signal handling for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down...")
		cancel()
	}()

	// Execute the custom tend application with our scenarios.
	if err := app.Execute(ctx, scenarios); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
