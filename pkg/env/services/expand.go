package services

import "os"

// ExpandEnvVars expands $VAR and ${VAR} references in s, looking up keys
// in envVars first and falling back to the process environment for keys
// not present in the map.
//
// This matches the historic daemon behavior of resolveEnvVars.
func ExpandEnvVars(s string, envVars map[string]string) string {
	return os.Expand(s, func(key string) string {
		if v, ok := envVars[key]; ok {
			return v
		}
		return os.Getenv(key)
	})
}
