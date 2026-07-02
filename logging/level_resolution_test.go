package logging

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestResolveLevels(t *testing.T) {
	tests := []struct {
		name        string
		env         string
		cfg         Config
		scope       LogScope
		wantConsole logrus.Level
		wantFile    logrus.Level
	}{
		{
			name:        "defaults to info for both sinks",
			cfg:         Config{},
			scope:       ScopeWorkspace,
			wantConsole: logrus.InfoLevel,
			wantFile:    logrus.InfoLevel,
		},
		{
			name:        "level applies to both sinks when file level unset",
			cfg:         Config{Level: "debug"},
			scope:       ScopeWorkspace,
			wantConsole: logrus.DebugLevel,
			wantFile:    logrus.DebugLevel,
		},
		{
			name:        "system_level wins in system scope",
			cfg:         Config{Level: "warn", SystemLevel: "debug"},
			scope:       ScopeSystem,
			wantConsole: logrus.DebugLevel,
			wantFile:    logrus.DebugLevel,
		},
		{
			name:        "system_level ignored in workspace scope",
			cfg:         Config{Level: "warn", SystemLevel: "debug"},
			scope:       ScopeWorkspace,
			wantConsole: logrus.WarnLevel,
			wantFile:    logrus.WarnLevel,
		},
		{
			name:        "file level more verbose than console",
			cfg:         Config{Level: "info", File: FileSinkConfig{Level: "debug"}},
			scope:       ScopeWorkspace,
			wantConsole: logrus.InfoLevel,
			wantFile:    logrus.DebugLevel,
		},
		{
			name:        "file level less verbose than console",
			cfg:         Config{Level: "debug", File: FileSinkConfig{Level: "warn"}},
			scope:       ScopeWorkspace,
			wantConsole: logrus.DebugLevel,
			wantFile:    logrus.WarnLevel,
		},
		{
			name:        "GROVE_LOG_LEVEL overrides both sinks",
			env:         "error",
			cfg:         Config{Level: "debug", SystemLevel: "debug", File: FileSinkConfig{Level: "debug"}},
			scope:       ScopeSystem,
			wantConsole: logrus.ErrorLevel,
			wantFile:    logrus.ErrorLevel,
		},
		{
			name:        "invalid levels fall back to info",
			cfg:         Config{Level: "nonsense", File: FileSinkConfig{Level: "bogus"}},
			scope:       ScopeWorkspace,
			wantConsole: logrus.InfoLevel,
			wantFile:    logrus.InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GROVE_LOG_LEVEL", tt.env)
			gotConsole, gotFile := resolveLevels(&tt.cfg, tt.scope)
			if gotConsole != tt.wantConsole {
				t.Errorf("consoleLevel = %v, want %v", gotConsole, tt.wantConsole)
			}
			if gotFile != tt.wantFile {
				t.Errorf("fileLevel = %v, want %v", gotFile, tt.wantFile)
			}
		})
	}
}

func TestMostVerbose(t *testing.T) {
	if got := mostVerbose(logrus.InfoLevel, logrus.DebugLevel); got != logrus.DebugLevel {
		t.Errorf("mostVerbose(info, debug) = %v, want debug", got)
	}
	if got := mostVerbose(logrus.DebugLevel, logrus.WarnLevel); got != logrus.DebugLevel {
		t.Errorf("mostVerbose(debug, warn) = %v, want debug", got)
	}
	if got := mostVerbose(logrus.WarnLevel, logrus.WarnLevel); got != logrus.WarnLevel {
		t.Errorf("mostVerbose(warn, warn) = %v, want warn", got)
	}
}

func TestLevelFilteringFormatter(t *testing.T) {
	inner := &TextFormatter{Config: FormatConfig{DisableTimestamp: true}}
	f := &levelFilteringFormatter{maxLevel: logrus.InfoLevel, inner: inner}

	debugEntry := &logrus.Entry{Level: logrus.DebugLevel, Message: "verbose detail"}
	out, err := f.Format(debugEntry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output for entry more verbose than maxLevel, got %q", out)
	}

	infoEntry := &logrus.Entry{Level: logrus.InfoLevel, Message: "visible message"}
	out, err = f.Format(infoEntry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected formatted output for entry at maxLevel")
	}

	warnEntry := &logrus.Entry{Level: logrus.WarnLevel, Message: "warning message"}
	out, err = f.Format(warnEntry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected formatted output for entry less verbose than maxLevel")
	}
}
