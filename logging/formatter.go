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

	level := strings.ToUpper(entry.Level.String())
	b.WriteString(fmt.Sprintf("[%s]", level))

	if component, ok := entry.Data["component"]; ok && !f.Config.DisableComponent {
		b.WriteString(fmt.Sprintf(" [%s]", component))
	}

	if entry.HasCaller() {
		// Show only filename and line number for brevity
		fileName := filepath.Base(entry.Caller.File)
		b.WriteString(fmt.Sprintf(" [%s:%d]", fileName, entry.Caller.Line))
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