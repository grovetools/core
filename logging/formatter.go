package logging

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// TextFormatter is a custom logrus formatter.
type TextFormatter struct {
	Config FormatConfig
}

// Format renders a single log entry.
func (f *TextFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b strings.Builder

	if !f.Config.DisableTimestamp {
		b.WriteString(entry.Time.Format("2006-01-02 15:04:05"))
		b.WriteString(" ")
	}

	// Map logrus level strings to shorter versions for consistency
	levelStr := entry.Level.String()
	switch levelStr {
	case "warning":
		levelStr = "warn"
	}
	level := strings.ToUpper(levelStr)
	b.WriteString(fmt.Sprintf("[%s]", level))

	if component, ok := entry.Data["component"]; ok && !f.Config.DisableComponent {
		b.WriteString(fmt.Sprintf(" [%s]", component))
	}

	if entry.HasCaller() {
		// Show filename, line number, and function name for enhanced debugging
		fileName := filepath.Base(entry.Caller.File)
		funcName := filepath.Base(entry.Caller.Function)
		b.WriteString(fmt.Sprintf(" [%s:%d %s]", fileName, entry.Caller.Line, funcName))
	}

	b.WriteString(" ")
	b.WriteString(entry.Message)

	// Append remaining fields
	for key, value := range entry.Data {
		if key != "component" {
			b.WriteString(fmt.Sprintf(" %s=%v", key, value))
		}
	}

	b.WriteString("\n")
	return []byte(b.String()), nil
}