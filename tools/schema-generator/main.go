package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/mattsolo1/grove-core/config"
)

func main() {
	schemaBytes, err := config.GenerateSchema()
	if err != nil {
		log.Fatalf("Error generating schema: %v", err)
	}

	// Define the output directory and ensure it exists.
	outputDir := "schema/definitions"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Error creating schema directory: %v", err)
	}

	// Write the schema to the file.
	outputPath := filepath.Join(outputDir, "base.schema.json")
	if err := os.WriteFile(outputPath, schemaBytes, 0644); err != nil {
		log.Fatalf("Error writing schema file: %v", err)
	}

	log.Printf("Successfully generated base schema at %s", outputPath)
}
