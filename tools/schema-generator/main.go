package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/theme"
)

func main() {
	// The tui.theme enum is generated from the embedded theme registry so
	// the schema roster can never drift from the data files.
	schemaBytes, err := config.GenerateSchemaWithThemeNames(theme.Names())
	if err != nil {
		log.Fatalf("Error generating schema: %v", err)
	}

	// Define the output directory and ensure it exists.
	outputDir := "schema/definitions"
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Fatalf("Error creating schema directory: %v", err)
	}

	// Write the schema to the file.
	outputPath := filepath.Join(outputDir, "base.schema.json")
	if err := os.WriteFile(outputPath, schemaBytes, 0o644); err != nil { //nolint:gosec // schema file is not sensitive
		log.Fatalf("Error writing schema file: %v", err)
	}

	log.Printf("Successfully generated base schema at %s", outputPath)
}
