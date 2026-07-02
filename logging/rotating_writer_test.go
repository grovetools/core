package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDateRotatingWriterMidnightRoll(t *testing.T) {
	dir := t.TempDir()
	current := time.Date(2026, 7, 1, 23, 59, 0, 0, time.UTC)
	nowFn := func() time.Time { return current }
	pathFn := func(now time.Time) string {
		return filepath.Join(dir, fmt.Sprintf("system-%s.log", now.Format("2006-01-02")))
	}

	w, err := newDateRotatingWriter(pathFn, nowFn)
	if err != nil {
		t.Fatalf("newDateRotatingWriter: %v", err)
	}

	if _, err := w.Write([]byte("before midnight\n")); err != nil {
		t.Fatalf("write before midnight: %v", err)
	}

	// Cross midnight.
	current = time.Date(2026, 7, 2, 0, 1, 0, 0, time.UTC)

	if _, err := w.Write([]byte("after midnight\n")); err != nil {
		t.Fatalf("write after midnight: %v", err)
	}

	day1, err := os.ReadFile(filepath.Join(dir, "system-2026-07-01.log"))
	if err != nil {
		t.Fatalf("reading day-1 file: %v", err)
	}
	if string(day1) != "before midnight\n" {
		t.Errorf("day-1 file = %q, want %q", day1, "before midnight\n")
	}

	day2, err := os.ReadFile(filepath.Join(dir, "system-2026-07-02.log"))
	if err != nil {
		t.Fatalf("reading day-2 file: %v", err)
	}
	if string(day2) != "after midnight\n" {
		t.Errorf("day-2 file = %q, want %q", day2, "after midnight\n")
	}

	if w.curPath != filepath.Join(dir, "system-2026-07-02.log") {
		t.Errorf("curPath = %q, want the day-2 path", w.curPath)
	}
}

func TestDateRotatingWriterSameDayNoRoll(t *testing.T) {
	dir := t.TempDir()
	current := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return current }
	pathFn := func(now time.Time) string {
		return filepath.Join(dir, fmt.Sprintf("system-%s.log", now.Format("2006-01-02")))
	}

	w, err := newDateRotatingWriter(pathFn, nowFn)
	if err != nil {
		t.Fatalf("newDateRotatingWriter: %v", err)
	}

	if _, err := w.Write([]byte("one\n")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	current = current.Add(5 * time.Hour) // later the same day
	if _, err := w.Write([]byte("two\n")); err != nil {
		t.Fatalf("second write: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one log file, got %d", len(entries))
	}

	content, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(content) != "one\ntwo\n" {
		t.Errorf("file content = %q, want %q", content, "one\ntwo\n")
	}
}

func TestDateRotatingWriterFixedPathNeverRolls(t *testing.T) {
	dir := t.TempDir()
	fixed := filepath.Join(dir, "fixed.log")
	current := time.Date(2026, 7, 1, 23, 59, 0, 0, time.UTC)
	nowFn := func() time.Time { return current }

	w, err := newDateRotatingWriter(func(time.Time) string { return fixed }, nowFn)
	if err != nil {
		t.Fatalf("newDateRotatingWriter: %v", err)
	}

	if _, err := w.Write([]byte("a\n")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	current = current.AddDate(0, 0, 3) // several days later
	if _, err := w.Write([]byte("b\n")); err != nil {
		t.Fatalf("second write: %v", err)
	}

	content, err := os.ReadFile(fixed)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(content) != "a\nb\n" {
		t.Errorf("file content = %q, want %q", content, "a\nb\n")
	}
}

func TestDateRotatingWriterCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	current := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return current }
	pathFn := func(now time.Time) string {
		return filepath.Join(dir, "nested", "deeper", fmt.Sprintf("workspace-%s.log", now.Format("2006-01-02")))
	}

	w, err := newDateRotatingWriter(pathFn, nowFn)
	if err != nil {
		t.Fatalf("newDateRotatingWriter: %v", err)
	}
	if _, err := w.Write([]byte("x\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(pathFn(current)); err != nil {
		t.Errorf("expected log file to exist in created directories: %v", err)
	}
}
