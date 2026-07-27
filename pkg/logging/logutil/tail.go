package logutil

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Tailer cadences. drainInterval is how often an open file is polled for new
// lines and sets the streaming latency floor. The other two govern directory
// scans (ReadDir plus a stat per candidate), which groved multiplies by one
// tailer per workspace — running those at the drain cadence made log tailing
// the daemon's top syscall source. Rotation is daily, and a workspace that
// starts logging only has to be noticed promptly, not instantly.
const (
	drainInterval         = 500 * time.Millisecond
	rotationCheckInterval = 15 * time.Second
	maxWaitInterval       = 60 * time.Second
)

// TailedLine represents a line of log output from a specific workspace.
// WorkspacePath is set for workspace-scoped lines (empty for system logs)
// so consumers that key on canonical paths (e.g. the embedded logs TUI
// filter) don't collide on duplicate workspace Names.
type TailedLine struct {
	Workspace     string
	WorkspacePath string
	Line          string
}

// Tail-lines sentinel semantics used by TailFile and TailDirectory:
//
//   - tailLines < 0 (e.g. -1): replay the full file from the beginning,
//     then (if follow) continue streaming. Preserves the historical
//     `core logs -f` behavior for callers that pass the legacy default.
//   - tailLines == 0: replay nothing — seek to EOF and stream only new
//     lines. This is the sensible default for interactive `-f` use,
//     where dumping an entire day's log on startup is jarring.
//   - tailLines > 0: replay the last N lines via readLastNLines (a
//     seek-from-end chunk walker — does NOT load the full file).
//
// Callers: core/cmd/logs.go translates the user-visible `--tail` flag
// into these values, defaulting to 0 when `-f` is set without an
// explicit `--tail` value (see runLogsE).

// readLastNLines seeks from the end of an open file and returns the
// last n complete lines (not counting a trailing empty line from a
// final newline). It reads in 8 KiB chunks backwards, stopping as
// soon as the chunk buffer contains more than n newlines. This lets
// callers tail gigabyte-sized log files without loading the whole
// thing into memory — the old implementation used io.ReadAll, which
// OOM'd on stale multi-month workspace logs.
func readLastNLines(f *os.File, n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := stat.Size()
	if size == 0 {
		return nil, nil
	}

	const chunkSize int64 = 8192
	var buf []byte
	pos := size
	for pos > 0 {
		readSize := chunkSize
		if pos < readSize {
			readSize = pos
		}
		pos -= readSize
		chunk := make([]byte, readSize)
		if _, err := f.ReadAt(chunk, pos); err != nil && err != io.EOF {
			return nil, err
		}
		buf = append(chunk, buf...)
		// We need n+1 newlines to know that the first line we kept
		// is a complete line (not a mid-line fragment from the first
		// chunk we read). Once we have that, we can stop.
		if bytes.Count(buf, []byte{'\n'}) > n {
			break
		}
	}

	lines := strings.Split(string(buf), "\n")
	// Drop a trailing empty string caused by the file ending in '\n'.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// TailFile reads a file and sends new lines to a channel. It is the
// stdlib-based tailer used by the non-TUI `core logs` command path —
// the interactive TUI uses hpcloud/tail for its richer rotation
// semantics. See the `Tail-lines sentinel semantics` comment above
// for the meaning of tailLines.
func TailFile(ctx context.Context, wsName, wsPath, path string, lineChan chan<- TailedLine, wg *sync.WaitGroup, follow bool, tailLines int) {
	defer wg.Done()

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	// Replay phase — emit the requested backlog before switching to
	// follow mode.
	switch {
	case tailLines < 0:
		// Legacy "full replay": stream from the start to EOF.
		reader := bufio.NewReader(f)
		for {
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				lineChan <- TailedLine{Workspace: wsName, WorkspacePath: wsPath, Line: strings.TrimSpace(line)}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return
			}
		}
	case tailLines == 0:
		// No replay — caller only wants new lines.
	default:
		// Bounded replay via seek-from-end.
		lines, err := readLastNLines(f, tailLines)
		if err == nil {
			for _, line := range lines {
				if line == "" {
					continue
				}
				lineChan <- TailedLine{Workspace: wsName, WorkspacePath: wsPath, Line: line}
			}
		}
	}

	if !follow {
		return
	}

	// Follow phase: seek to the current end and poll for new lines.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return
	}
	reader := bufio.NewReader(f)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			lineChan <- TailedLine{Workspace: wsName, WorkspacePath: wsPath, Line: strings.TrimSpace(line)}
		}
		if err == io.EOF {
			time.Sleep(drainInterval)
			continue
		}
		if err != nil {
			return
		}
	}
}

// TailDirectory watches a log directory for files and tails them.
// It handles the case where the directory or files don't exist yet.
// See the `Tail-lines sentinel semantics` comment above for the
// meaning of tailLines.
func TailDirectory(ctx context.Context, wsName, wsPath, logsDir string, lineChan chan<- TailedLine, wg *sync.WaitGroup, follow bool, tailLines int) {
	defer wg.Done()

	var currentFile string
	var f *os.File

	// Wait for directory and files to appear. Most workspaces in a large
	// portfolio never write a log at all, so this loop is where the majority
	// of tailers live permanently; polling every drain interval turned that
	// into a continuous storm of failing ReadDir calls. Back off instead —
	// a workspace that starts logging is still picked up within a minute.
	waitInterval := drainInterval
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		logFile, err := FindLatestLogFile(logsDir)
		if err == nil {
			currentFile = logFile
			break
		}
		if !follow {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(waitInterval):
		}
		if waitInterval < maxWaitInterval {
			waitInterval *= 2
			if waitInterval > maxWaitInterval {
				waitInterval = maxWaitInterval
			}
		}
	}

	f, err := os.Open(currentFile)
	if err != nil {
		return
	}

	// Replay phase.
	switch {
	case tailLines < 0:
		reader := bufio.NewReader(f)
		for {
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				lineChan <- TailedLine{Workspace: wsName, WorkspacePath: wsPath, Line: strings.TrimSpace(line)}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				f.Close()
				return
			}
		}
	case tailLines == 0:
		// No replay.
	default:
		lines, err := readLastNLines(f, tailLines)
		if err == nil {
			for _, line := range lines {
				if line == "" {
					continue
				}
				lineChan <- TailedLine{Workspace: wsName, WorkspacePath: wsPath, Line: line}
			}
		}
	}

	if !follow {
		f.Close()
		return
	}

	// Seek to end for follow phase.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return
	}
	reader := bufio.NewReader(f)

	checkInterval := time.NewTicker(drainInterval)
	defer checkInterval.Stop()

	// Rotation is checked on a much slower cadence than draining. Each check
	// is a ReadDir plus a stat per candidate, and groved runs one tailer per
	// workspace — at the drain cadence that was hundreds of directory scans a
	// second across the portfolio, for files that rotate daily. Detecting a
	// rotation late costs latency only: the drain above always runs the
	// current file to EOF first, and the new file is read from its start.
	lastRotationCheck := time.Now()

	for {
		select {
		case <-ctx.Done():
			f.Close()
			return
		default:
		}

		// Drain available lines.
		for {
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				lineChan <- TailedLine{Workspace: wsName, WorkspacePath: wsPath, Line: strings.TrimSpace(line)}
			}
			if err != nil {
				break
			}
		}

		select {
		case <-ctx.Done():
			f.Close()
			return
		case <-checkInterval.C:
		}

		// Check for newer log file (daily rotation).
		if time.Since(lastRotationCheck) < rotationCheckInterval {
			continue
		}
		lastRotationCheck = time.Now()
		latestFile, err := FindLatestLogFile(logsDir)
		if err == nil && latestFile != currentFile {
			f.Close()
			currentFile = latestFile
			f, err = os.Open(currentFile)
			if err != nil {
				return
			}
			// Start from the beginning of the new file so we don't
			// miss any lines written between rotation and our switch.
			reader = bufio.NewReader(f)
		}
	}
}
