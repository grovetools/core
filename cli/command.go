package cli

import (
    "os"
    
    "github.com/spf13/cobra"
    "github.com/sirupsen/logrus"
    "github.com/mattsolo1/grove-core/config"
    "github.com/mattsolo1/grove-core/logging"
)

// CommandOptions holds common options for Grove commands
type CommandOptions struct {
    ConfigFile string
    Verbose    bool
    JSONOutput bool
}

// NewStandardCommand creates a new command with standard Grove flags
func NewStandardCommand(use, short string) *cobra.Command {
    cmd := &cobra.Command{
        Use:   use,
        Short: short,
    }

    // Standard flags for all Grove tools
    cmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose logging")
    cmd.PersistentFlags().Bool("json", false, "Output in JSON format")
    cmd.PersistentFlags().StringP("config", "c", "", "Path to grove.yml config file")

    // Apply styled help
    SetStyledHelp(cmd)

    return cmd
}

// GetLogger creates a logger based on command flags
func GetLogger(cmd *cobra.Command) *logrus.Logger {
    // Use grove-core logging which is already configured
    // This returns a logrus.Entry, we need to get the underlying logger
    entry := logging.NewLogger("grove-cli")
    logger := entry.Logger
    
    verbose, _ := cmd.Flags().GetBool("verbose")
    if verbose {
        logger.SetLevel(logrus.DebugLevel)
    }
    
    jsonOutput, _ := cmd.Flags().GetBool("json")
    if jsonOutput {
        logger.SetFormatter(&logrus.JSONFormatter{})
    }
    
    return logger
}

// GetOptions extracts common options from a command
func GetOptions(cmd *cobra.Command) CommandOptions {
    configFile, _ := cmd.Flags().GetString("config")
    verbose, _ := cmd.Flags().GetBool("verbose")
    jsonOutput, _ := cmd.Flags().GetBool("json")
    
    return CommandOptions{
        ConfigFile: configFile,
        Verbose:    verbose,
        JSONOutput: jsonOutput,
    }
}

// InitConfig initializes the configuration file path
func InitConfig(configFile string) (string, error) {
    if configFile != "" {
        // Use config file from flag
        return configFile, nil
    }

    // Find config file
    cwd, err := os.Getwd()
    if err != nil {
        return "", err
    }

    foundConfigFile, err := config.FindConfigFile(cwd)
    if err != nil {
        // No config file found, that's okay for some commands
        return "", nil
    }

    return foundConfigFile, nil
}