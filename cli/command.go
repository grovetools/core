package cli

import (
    "github.com/spf13/cobra"
    "github.com/sirupsen/logrus"
)

func NewStandardCommand(use, short string) *cobra.Command {
    cmd := &cobra.Command{
        Use:   use,
        Short: short,
    }
    
    // Standard flags for all Grove tools
    cmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose logging")
    cmd.PersistentFlags().Bool("json", false, "Output in JSON format")
    cmd.PersistentFlags().StringP("config", "c", "", "Path to grove.yml config file")
    
    return cmd
}

func GetLogger(cmd *cobra.Command) *logrus.Logger {
    logger := logrus.New()
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