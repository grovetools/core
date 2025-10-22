package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	groveSchema "github.com/mattsolo1/grove-core/schema"
)

func main() {
	log.Println("Starting schema composition...")

	baseSchemaPath := "schema/definitions/base.schema.json"
	distDir := "schema/dist"

	// Ensure dist directory exists
	if err := os.MkdirAll(distDir, 0755); err != nil {
		log.Fatalf("Failed to create dist directory: %v", err)
	}

	// 1. Generate the resolvable schema (with remote $refs) for IDEs.
	resolvableSchema, err := createResolvableSchema(baseSchemaPath)
	if err != nil {
		log.Fatalf("Failed to create resolvable schema: %v", err)
	}
	resolvablePath := filepath.Join(distDir, "grove.schema.json")
	if err := writeJSONFile(resolvablePath, resolvableSchema); err != nil {
		log.Fatalf("Failed to write resolvable schema: %v", err)
	}
	log.Printf("Generated resolvable schema at %s", resolvablePath)

	// 2. Generate the bundled schema (with resolved $refs) for embedding.
	bundledSchema, err := createBundledSchema(resolvableSchema)
	if err != nil {
		log.Fatalf("Failed to create bundled schema: %v", err)
	}
	bundledPath := filepath.Join(distDir, "grove.embedded.schema.json")
	if err := writeJSONFile(bundledPath, bundledSchema); err != nil {
		log.Fatalf("Failed to write bundled schema: %v", err)
	}
	log.Printf("Generated bundled schema at %s", bundledPath)

	log.Println("Schema composition complete.")
}

func createResolvableSchema(basePath string) (map[string]interface{}, error) {
	baseBytes, err := os.ReadFile(basePath)
	if err != nil {
		return nil, fmt.Errorf("could not read base schema: %w", err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(baseBytes, &schema); err != nil {
		return nil, fmt.Errorf("could not parse base schema: %w", err)
	}

	// Ensure properties map exists
	if _, ok := schema["properties"]; !ok {
		schema["properties"] = make(map[string]interface{})
	}
	properties := schema["properties"].(map[string]interface{})

	// Add extension properties with remote $ref
	for key, url := range groveSchema.ExtensionSchemaURLs {
		properties[key] = map[string]interface{}{
			"$ref": url,
		}
	}

	// Set additionalProperties to true to allow extension properties
	schema["additionalProperties"] = true
	schema["title"] = "Grove Ecosystem Configuration Schema"
	schema["description"] = "A unified schema for all grove.yml configuration files."

	return schema, nil
}

func createBundledSchema(resolvableSchema map[string]interface{}) (map[string]interface{}, error) {
	bundledSchema := deepCopyMap(resolvableSchema)

	// If there are no extension schemas to fetch, just return the base schema
	if len(groveSchema.ExtensionSchemaURLs) == 0 {
		return bundledSchema, nil
	}

	properties := bundledSchema["properties"].(map[string]interface{})

	var wg sync.WaitGroup
	errs := make(chan error, len(groveSchema.ExtensionSchemaURLs))
	var mu sync.Mutex

	for key, url := range groveSchema.ExtensionSchemaURLs {
		wg.Add(1)
		go func(key, url string) {
			defer wg.Done()
			log.Printf("Fetching schema for '%s' from %s", key, url)

			resp, err := http.Get(url)
			if err != nil {
				errs <- fmt.Errorf("failed to fetch schema for %s: %w", key, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errs <- fmt.Errorf("bad status fetching schema for %s: %s", key, resp.Status)
				return
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				errs <- fmt.Errorf("failed to read schema body for %s: %w", key, err)
				return
			}

			var subSchema map[string]interface{}
			if err := json.Unmarshal(body, &subSchema); err != nil {
				errs <- fmt.Errorf("failed to parse schema for %s: %w", key, err)
				return
			}

			mu.Lock()
			properties[key] = subSchema
			mu.Unlock()
		}(key, url)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		return nil, err
	}

	return bundledSchema, nil
}

func writeJSONFile(path string, data map[string]interface{}) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0644)
}

func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	// Simple deep copy using JSON marshaling
	bytes, err := json.Marshal(m)
	if err != nil {
		return m
	}
	var copy map[string]interface{}
	if err := json.Unmarshal(bytes, &copy); err != nil {
		return m
	}
	return copy
}
