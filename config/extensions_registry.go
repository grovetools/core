package config

import "sort"

// ExtensionInfo describes a known extension namespace: a top-level config key
// that is not a core Config field but is consumed by a grovetools repo via
// Config.UnmarshalExtension (or equivalent raw access). The registry is the
// audit's source of truth for "some code reads this key" — nested keys under a
// registered namespace are owned by the consuming code and are not classified
// further.
type ExtensionInfo struct {
	// Key is the top-level config key (e.g. "flow" for [flow] blocks).
	Key string
	// Repo is the grovetools repo that owns/consumes the namespace.
	Repo string
	// Description is a short human-readable summary of what the namespace
	// configures.
	Description string
}

// knownExtensions maps an extension key to its metadata. Seeded with the
// verified consumers across the ecosystem (every key here has at least one
// UnmarshalExtension call site or equivalent reader). Downstream repos linked
// into one binary can add their own entries at init via RegisterExtension.
var knownExtensions = map[string]ExtensionInfo{
	"flow":          {Key: "flow", Repo: "flow", Description: "Flow orchestration (plans, jobs, models)"},
	"hooks":         {Key: "hooks", Repo: "hooks", Description: "Lifecycle hooks (on_stop, async hooks, plan preservation)"},
	"notifications": {Key: "notifications", Repo: "notify", Description: "Desktop/remote notification routing"},
	"skills":        {Key: "skills", Repo: "skills", Description: "Skill library configuration"},
	"playbooks":     {Key: "playbooks", Repo: "skills", Description: "Playbook definitions"},
	"binary":        {Key: "binary", Repo: "tend", Description: "Binary/tool discovery for test runners"},
	"anthropic":     {Key: "anthropic", Repo: "grove-anthropic", Description: "Anthropic API access (key sources)"},
	"claude":        {Key: "claude", Repo: "grove-anthropic", Description: "Claude Code settings profile (also read by core/pkg/claudenotebook)"},
	"logging":       {Key: "logging", Repo: "core", Description: "Structured logging (levels, sinks)"},
	"keys":          {Key: "keys", Repo: "core", Description: "Global keybinding registry (core/pkg/keybind, grove keys)"},
	"nav":           {Key: "nav", Repo: "nav", Description: "Session/window navigation groups"},
	"llm":           {Key: "llm", Repo: "grove", Description: "LLM provider/model selection for CLI helpers"},
	"context":       {Key: "context", Repo: "cx", Description: "cx context tool settings (also a core Config field; core takes precedence)"},
	"aglogs":        {Key: "aglogs", Repo: "agentlogs", Description: "Agent session log reader configuration"},
	"gemini":        {Key: "gemini", Repo: "grove-gemini", Description: "Gemini API access (key sources)"},
	"tmux":          {Key: "tmux", Repo: "nav", Description: "tmux integration settings"},
	"description":   {Key: "description", Repo: "grove", Description: "Repo description (registry-generator metadata)"},
	"managed":       {Key: "managed", Repo: "grove", Description: "Repo managed flag (registry-generator metadata)"},
}

// RegisterExtension registers (or overrides) the metadata for an extension
// key. Intended for downstream packages that consume their own [extension]
// block to self-register at init, mirroring RegisterExtensionMergePolicy.
func RegisterExtension(info ExtensionInfo) {
	knownExtensions[info.Key] = info
}

// KnownExtension looks up the registered metadata for an extension key.
func KnownExtension(key string) (ExtensionInfo, bool) {
	info, ok := knownExtensions[key]
	return info, ok
}

// KnownExtensions returns all registered extensions sorted by key.
func KnownExtensions() []ExtensionInfo {
	out := make([]ExtensionInfo, 0, len(knownExtensions))
	for _, info := range knownExtensions {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
