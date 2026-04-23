// Package services holds pure-functional parsing helpers for the per-service
// spawn config that the daemon consumes from grove.toml.
//
// The daemon drives the state machines (process supervision, goroutines,
// port allocation, proxy wiring); this package only turns raw
// map[string]interface{} config into typed structs and provides small
// stateless helpers (env expansion, TCP probe) used by those machines.
package services

// ServiceEntry is one parsed service from the `services` config map.
//
// Fields correspond directly to the keys the daemon reads out of svcConfig
// today. Unmodeled keys remain accessible via Raw so daemon-specific logic
// (docker args, cleanup_paths, etc.) can still introspect the original map.
type ServiceEntry struct {
	Name        string
	Type        string
	Command     string
	WorkingDir  string
	PortEnv     string
	Route       string
	Order       int
	Env         map[string]interface{}
	Volumes     map[string]interface{}
	Lifecycle   *LifecycleConfig
	HealthCheck *HealthCheckConfig
	Raw         map[string]interface{}
}

// LifecycleConfig is the parsed form of svcConfig["lifecycle"].
type LifecycleConfig struct {
	PostStart     string
	PostStartMode string // "always" (default) or "once"
}

// HealthCheckConfig is the parsed form of svcConfig["health_check"].
type HealthCheckConfig struct {
	Type           string // "tcp" is the only type currently supported
	TimeoutSeconds int    // default 30
}
