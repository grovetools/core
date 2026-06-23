package git

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/grovetools/core/command"
)

// LogEntry is a single commit from `git log`, parsed for the git-viewer Log
// page. Date is the author date (parsed from strict ISO-8601); it is the zero
// time.Time when the date field is missing or unparseable.
type LogEntry struct {
	Hash    string    `json:"hash"`
	Author  string    `json:"author"`
	Email   string    `json:"email"`
	Date    time.Time `json:"date"`
	Subject string    `json:"subject"`
}

// GetLog returns up to limit commits from HEAD of the repository at repoPath,
// most recent first. A limit <= 0 returns the full history.
//
// It runs `git log -z --format=...` with a Unit-Separator (0x1f) between fields
// and a NUL between records, so subjects containing spaces, tabs, or newlines
// parse unambiguously. An empty repository (no commits yet) is not an error:
// GetLog returns (nil, nil), matching GetChangedFiles' treatment of the same
// state.
func GetLog(repoPath string, limit int) ([]LogEntry, error) {
	cmdBuilder := command.NewSafeBuilder()
	// %H hash, %an author name, %ae author email, %aI author date (ISO-8601
	// strict, RFC3339-parseable), %s subject — Unit-Separator delimited.
	args := []string{"log", "-z", "--format=%H%x1f%an%x1f%ae%x1f%aI%x1f%s"}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}

	cmd, err := cmdBuilder.Build(context.Background(), "git", args...)
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath

	var stderr bytes.Buffer
	execCmd.Stderr = &stderr
	output, err := execCmd.Output()
	if err != nil {
		errStr := stderr.String()
		// A fresh repo before its first commit has no log to show.
		if strings.Contains(errStr, "does not have any commits yet") ||
			strings.Contains(errStr, "No commits yet") {
			return nil, nil
		}
		if strings.Contains(errStr, "not a git repository") {
			return nil, fmt.Errorf("not a git repository: %s", repoPath)
		}
		return nil, fmt.Errorf("failed to get git log: %w, output: %s", err, errStr)
	}

	return parseLog(string(output)), nil
}

// parseLog parses NUL-delimited, Unit-Separator-fielded `git log` output into
// LogEntry values. It is split out from GetLog so it can be unit-tested without
// invoking git. Records with fewer than five fields are skipped defensively.
func parseLog(output string) []LogEntry {
	records := strings.Split(output, "\x00")
	var entries []LogEntry

	for _, record := range records {
		if record == "" {
			continue
		}
		fields := strings.Split(record, "\x1f")
		if len(fields) < 5 {
			continue
		}
		entry := LogEntry{
			Hash:    fields[0],
			Author:  fields[1],
			Email:   fields[2],
			Subject: fields[4],
		}
		if t, err := time.Parse(time.RFC3339, fields[3]); err == nil {
			entry.Date = t
		}
		entries = append(entries, entry)
	}

	return entries
}
