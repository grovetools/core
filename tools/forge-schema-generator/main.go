package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/grovetools/core/config"
	"github.com/invopop/jsonschema"
)

func main() {
	r := &jsonschema.Reflector{
		AllowAdditionalProperties: false,
		ExpandedStruct:            true,
		FieldNameTag:              "yaml",
	}
	schema := r.Reflect(&config.ForgeConfig{})
	schema.Title = "Grove Forge Configuration"
	schema.Description = "The typed [forge] extension namespace."
	raw, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	path := filepath.Join("forge.schema.json")
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
}
