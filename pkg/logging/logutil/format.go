package logutil

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
)

// FormatLogLine formats a parsed log entry in the specified format.
// Supported formats: text, json, full, rich, pretty, pretty-text.
// compact controls whether blank lines are added between entries.
func FormatLogLine(logMap map[string]interface{}, workspace string, format string, compact bool) string {
	switch format {
	case "json":
		return formatJSON(logMap, workspace)
	case "pretty":
		return formatPretty(logMap, true, compact)
	case "pretty-text":
		return formatPretty(logMap, false, compact)
	case "full":
		return formatFull(logMap, workspace, compact)
	case "rich":
		return formatRich(logMap, compact)
	default: // "text"
		return formatText(logMap, workspace)
	}
}

// styleLevelStr returns a styled level string.
func styleLevelStr(level string) string {
	var levelStyle lipgloss.Style
	switch strings.ToLower(level) {
	case "error", "fatal", "panic":
		levelStyle = theme.DefaultTheme.Error
	case "warning":
		levelStyle = theme.DefaultTheme.Warning
	case "info":
		levelStyle = theme.DefaultTheme.Info
	default:
		levelStyle = theme.DefaultTheme.Muted
	}
	return levelStyle.Render(strings.ToUpper(level))
}

// parseTimeStr extracts and formats a time string from a log map.
func parseTimeStr(logMap map[string]interface{}) string {
	ts, _ := logMap["time"].(string)
	parsedTime, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		parsedTime, _ = time.Parse(time.RFC3339, ts)
	}
	return parsedTime.Format("15:04:05")
}

// excludeStandardFields is the set of fields excluded from "other fields" display.
var excludeStandardFields = map[string]bool{
	"time": true, "level": true, "msg": true, "component": true,
	"workspace": true, "pretty_ansi": true, "pretty_text": true,
}

// formatOtherFields returns a formatted string of non-standard fields.
func formatOtherFields(logMap map[string]interface{}) string {
	var sortedKeys []string
	for k := range logMap {
		if !excludeStandardFields[k] {
			sortedKeys = append(sortedKeys, k)
		}
	}
	sort.Strings(sortedKeys)

	var parts []string
	for _, k := range sortedKeys {
		parts = append(parts, fmt.Sprintf("%s=%v", theme.DefaultTheme.Muted.Render(k), logMap[k]))
	}
	return strings.Join(parts, " ")
}

func formatJSON(logMap map[string]interface{}, workspace string) string {
	// Filter out pretty_ansi (ANSI codes don't belong in JSON output)
	out := make(map[string]interface{}, len(logMap))
	for k, v := range logMap {
		if k != "pretty_ansi" {
			out[k] = v
		}
	}
	out["workspace"] = workspace
	jsonData, _ := json.Marshal(out)
	return string(jsonData)
}

func formatPretty(logMap map[string]interface{}, withANSI bool, compact bool) string {
	var prettyOutput string
	if withANSI {
		prettyOutput, _ = logMap["pretty_ansi"].(string)
	} else {
		prettyOutput, _ = logMap["pretty_text"].(string)
	}
	if prettyOutput == "" {
		prettyOutput, _ = logMap["msg"].(string)
	}
	if prettyOutput == "" {
		return ""
	}
	if !compact {
		return prettyOutput + "\n"
	}
	return prettyOutput
}

func formatFull(logMap map[string]interface{}, workspace string, compact bool) string {
	timeStr := parseTimeStr(logMap)
	level, _ := logMap["level"].(string)
	msg, _ := logMap["msg"].(string)
	component, _ := logMap["component"].(string)

	fieldsStr := formatOtherFields(logMap)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s [%s] %s %s [%s] %s\n",
		timeStr,
		theme.DefaultTheme.Accent.Render(workspace),
		styleLevelStr(level),
		msg,
		theme.DefaultTheme.Muted.Render(component),
		fieldsStr,
	)

	if prettyAnsi, ok := logMap["pretty_ansi"].(string); ok && prettyAnsi != "" {
		fmt.Fprintf(&sb, "         %s %s\n", theme.DefaultTheme.Muted.Render("└─"), prettyAnsi)
	}
	if !compact {
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatRich(logMap map[string]interface{}, compact bool) string {
	timeStr := parseTimeStr(logMap)
	level, _ := logMap["level"].(string)
	component, _ := logMap["component"].(string)

	prettyOutput := ""
	if v, ok := logMap["pretty_ansi"].(string); ok && v != "" {
		prettyOutput = v
	} else if msg, ok := logMap["msg"].(string); ok {
		prettyOutput = msg
	}

	var sb strings.Builder
	isMultiLine := strings.Contains(prettyOutput, "\n")
	if isMultiLine {
		fmt.Fprintf(&sb, "%s [%s] %s\n%s\n",
			theme.DefaultTheme.Muted.Render(timeStr),
			theme.DefaultTheme.Muted.Render(component),
			styleLevelStr(level),
			prettyOutput,
		)
	} else {
		fmt.Fprintf(&sb, "%s [%s] %s %s\n",
			theme.DefaultTheme.Muted.Render(timeStr),
			theme.DefaultTheme.Muted.Render(component),
			styleLevelStr(level),
			prettyOutput,
		)
	}
	if !compact {
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatText(logMap map[string]interface{}, workspace string) string {
	timeStr := parseTimeStr(logMap)
	level, _ := logMap["level"].(string)
	msg, _ := logMap["msg"].(string)
	component, _ := logMap["component"].(string)

	fieldsStr := formatOtherFields(logMap)

	return fmt.Sprintf("%s [%s] %s %s [%s] %s\n",
		timeStr,
		theme.DefaultTheme.Accent.Render(workspace),
		styleLevelStr(level),
		msg,
		theme.DefaultTheme.Muted.Render(component),
		fieldsStr,
	)
}
