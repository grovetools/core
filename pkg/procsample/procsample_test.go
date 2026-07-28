package procsample

import (
	"errors"
	"testing"
	"time"
)

const darwinFixture = `    1     0   0.0  21584 12:41.61 /sbin/launchd
  380     1   0.3  54321 1:02:03 /usr/libexec/logd
27462     1  41.3 727040 25:01.99 /opt/homebrew/bin/groved
27500 27462   2.5  10240  0:00.12 git
  600     1   1.2 512000  3:15.07 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome Helper (Renderer)
`

const linuxFixture = `      1       0  0.0 11840 00:00:04 systemd
      2       0  0.0     0 00:00:00 kthreadd
   1234       1  1.5 204800 0-01:02:03 groved
   1300    1234 12.0 51200 00:10:30.55 nvim
   1301    1300  0.0  8192 1-02:03:04 hash-object storm
`

func withRunPS(t *testing.T, out string, err error) {
	t.Helper()
	orig := runPS
	runPS = func() ([]byte, error) { return []byte(out), err }
	t.Cleanup(func() { runPS = orig })
}

func TestSnapshotDarwin(t *testing.T) {
	withRunPS(t, darwinFixture, nil)
	procs, err := Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(procs) != 5 {
		t.Fatalf("got %d procs, want 5", len(procs))
	}

	launchd := procs[1]
	if launchd.PPID != 0 || launchd.Comm != "launchd" || launchd.RSSKB != 21584 {
		t.Errorf("launchd = %+v", launchd)
	}
	if want := 12*time.Minute + 41*time.Second + 610*time.Millisecond; launchd.CPUTime != want {
		t.Errorf("launchd.CPUTime = %v, want %v", launchd.CPUTime, want)
	}

	logd := procs[380]
	if want := time.Hour + 2*time.Minute + 3*time.Second; logd.CPUTime != want {
		t.Errorf("logd.CPUTime = %v, want %v", logd.CPUTime, want)
	}

	groved := procs[27462]
	if groved.PctCPU != 41.3 || groved.Comm != "groved" {
		t.Errorf("groved = %+v", groved)
	}

	git := procs[27500]
	if git.PPID != 27462 || git.Comm != "git" || git.CPUTime != 120*time.Millisecond {
		t.Errorf("git = %+v", git)
	}

	// comm with spaces: last field runs to end of line, basename applied.
	chrome := procs[600]
	if chrome.Comm != "Google Chrome Helper (Renderer)" {
		t.Errorf("chrome.Comm = %q", chrome.Comm)
	}
}

func TestSnapshotLinux(t *testing.T) {
	withRunPS(t, linuxFixture, nil)
	procs, err := Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(procs) != 5 {
		t.Fatalf("got %d procs, want 5", len(procs))
	}

	groved := procs[1234]
	if want := time.Hour + 2*time.Minute + 3*time.Second; groved.CPUTime != want {
		t.Errorf("groved.CPUTime = %v, want %v (days form)", groved.CPUTime, want)
	}

	nvim := procs[1300]
	if want := 10*time.Minute + 30*time.Second + 550*time.Millisecond; nvim.CPUTime != want {
		t.Errorf("nvim.CPUTime = %v, want %v (fractional)", nvim.CPUTime, want)
	}

	storm := procs[1301]
	if storm.Comm != "hash-object storm" {
		t.Errorf("storm.Comm = %q", storm.Comm)
	}
	if want := 26*time.Hour + 3*time.Minute + 4*time.Second; storm.CPUTime != want {
		t.Errorf("storm.CPUTime = %v, want %v (days form)", storm.CPUTime, want)
	}
}

func TestSnapshotSkipsMalformedLines(t *testing.T) {
	withRunPS(t, "garbage line\n  100 1 0.0 512 0:01.00 ok\n1 2 3\n\n", nil)
	procs, err := Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(procs) != 1 || procs[100].Comm != "ok" {
		t.Fatalf("procs = %+v, want just pid 100", procs)
	}
}

func TestSnapshotPSError(t *testing.T) {
	withRunPS(t, "", errors.New("boom"))
	if _, err := Snapshot(); err == nil {
		t.Fatal("expected error when ps fails")
	}
}

func TestParseCPUTime(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{in: "05", want: 5 * time.Second},
		{in: "0:00.12", want: 120 * time.Millisecond},
		{in: "12:41.61", want: 12*time.Minute + 41*time.Second + 610*time.Millisecond},
		{in: "123:45.67", want: 123*time.Minute + 45*time.Second + 670*time.Millisecond},
		{in: "1:02:03", want: time.Hour + 2*time.Minute + 3*time.Second},
		{in: "00:00:04", want: 4 * time.Second},
		{in: "0-01:02:03", want: time.Hour + 2*time.Minute + 3*time.Second},
		{in: "1-02:03:04", want: 26*time.Hour + 3*time.Minute + 4*time.Second},
		{in: "10-00:00:00.50", want: 240*time.Hour + 500*time.Millisecond},
		{in: "abc", wantErr: true},
		{in: "1:2:3:4", wantErr: true},
		{in: "-1:00", wantErr: true},
		{in: "x-01:02:03", wantErr: true},
		{in: "1:.5", wantErr: true},
	}
	for _, tc := range cases {
		got, err := parseCPUTime(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseCPUTime(%q) = %v, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseCPUTime(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseCPUTime(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
