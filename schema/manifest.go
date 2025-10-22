package schema

// ExtensionSchemaURLs maps Grove extension keys to the canonical URL of their JSON schema.
// Tools in the ecosystem publish their own schemas, and this manifest is used to compose them
// into a unified schema for validation and IDE support.
//
// NOTE: For now using placeholder GitHub release URLs. In production, these should redirect
// through schemas.grove.sh for clean, versioned URLs.
//
// Extension schemas are currently commented out for testing. Once schemas are published as
// GitHub release assets or through a schema hosting service, they can be uncommented.
var ExtensionSchemaURLs = map[string]string{
	// "proxy": "https://github.com/mattsolo1/grove-proxy/releases/download/v0.4.0/proxy.schema.json",
	// Additional extensions will be added here as they publish their schemas:
	// "flow":   "https://schemas.grove.sh/flow/v1.schema.json",
	// "gemini": "https://schemas.grove.sh/gemini/v1.schema.json",
	// "hooks":  "https://schemas.grove.sh/hooks/v1.schema.json",
}
