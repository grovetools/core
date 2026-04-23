package services

import "sort"

// ParseAndSort extracts services from config["services"] and returns them
// sorted by ascending Order, breaking ties alphabetically by name.
//
// A missing or malformed services block returns an empty slice (not an
// error) — the caller decides whether that's valid.
func ParseAndSort(config map[string]interface{}) []ServiceEntry {
	raw, ok := config["services"].(map[string]interface{})
	if !ok {
		return nil
	}

	entries := make([]ServiceEntry, 0, len(raw))
	for name, svcRaw := range raw {
		svcConfig, ok := svcRaw.(map[string]interface{})
		if !ok {
			continue
		}
		entries = append(entries, parseEntry(name, svcConfig))
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Order != entries[j].Order {
			return entries[i].Order < entries[j].Order
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}

func parseEntry(name string, svcConfig map[string]interface{}) ServiceEntry {
	entry := ServiceEntry{
		Name: name,
		Raw:  svcConfig,
	}
	entry.Type, _ = svcConfig["type"].(string)
	entry.Command, _ = svcConfig["command"].(string)
	entry.WorkingDir, _ = svcConfig["working_dir"].(string)
	entry.PortEnv, _ = svcConfig["port_env"].(string)
	entry.Route, _ = svcConfig["route"].(string)
	entry.Order = parseOrder(svcConfig["order"])
	entry.Env, _ = svcConfig["env"].(map[string]interface{})
	entry.Volumes, _ = svcConfig["volumes"].(map[string]interface{})
	entry.Lifecycle = ParseLifecycle(svcConfig["lifecycle"])
	entry.HealthCheck = ParseHealthCheck(svcConfig["health_check"])
	return entry
}

func parseOrder(v interface{}) int {
	// Default: 100 — high enough that explicitly-ordered services sort first.
	switch o := v.(type) {
	case int64:
		return int(o)
	case float64:
		return int(o)
	case int:
		return o
	}
	return 100
}

// ParseLifecycle converts a lifecycle config map into a typed struct.
// Returns nil when the input is missing, malformed, or has no post_start.
func ParseLifecycle(v interface{}) *LifecycleConfig {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	postStart, _ := m["post_start"].(string)
	if postStart == "" {
		return nil
	}
	mode, _ := m["post_start_mode"].(string)
	if mode == "" {
		mode = "always"
	}
	return &LifecycleConfig{PostStart: postStart, PostStartMode: mode}
}

// ParseHealthCheck converts a health_check config map into a typed struct.
// Returns nil when the input is missing, malformed, or has an unsupported type.
func ParseHealthCheck(v interface{}) *HealthCheckConfig {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	hcType, _ := m["type"].(string)
	if hcType == "" {
		hcType = "tcp"
	}
	timeout := 30
	if ts, ok := m["timeout_seconds"].(int64); ok {
		timeout = int(ts)
	} else if ts, ok := m["timeout_seconds"].(float64); ok {
		timeout = int(ts)
	} else if ts, ok := m["timeout_seconds"].(int); ok {
		timeout = ts
	}
	return &HealthCheckConfig{Type: hcType, TimeoutSeconds: timeout}
}
