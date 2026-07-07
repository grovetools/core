package castwriter

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
)

// Real-world sample casts maintained in the notebook. These paths may not exist
// in CI or on a fresh checkout, so the tests skip gracefully when unreachable.
var sampleCasts = map[string]int{
	"/Users/solair/notebooks/grovetools/workspaces/flow/docgen/asciicasts/01-plan-init.cast": 3,
	"/Users/solair/notebooks/grovetools/workspaces/flow/docgen/asciicasts/test.cast":         2,
}

func TestParseRealSampleHeaders(t *testing.T) {
	reached := false
	for path, wantVersion := range sampleCasts {
		f, err := os.Open(path)
		if err != nil {
			t.Logf("skipping unreachable sample %s: %v", path, err)
			continue
		}
		reached = true
		func() {
			defer f.Close()
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
			if !sc.Scan() {
				t.Errorf("%s: empty file", path)
				return
			}
			var hdr map[string]any
			if err := json.Unmarshal(sc.Bytes(), &hdr); err != nil {
				t.Errorf("%s: header not valid JSON: %v", path, err)
				return
			}
			v, _ := hdr["version"].(float64)
			if int(v) != wantVersion {
				t.Errorf("%s: version = %v, want %d", path, v, wantVersion)
			}
			// v3 carries dimensions under "term"; v2 has top-level width/height.
			if wantVersion == 3 {
				term, ok := hdr["term"].(map[string]any)
				if !ok {
					t.Errorf("%s: v3 header missing term object", path)
					return
				}
				if _, ok := term["cols"]; !ok {
					t.Errorf("%s: v3 term missing cols", path)
				}
			} else {
				if _, ok := hdr["width"]; !ok {
					t.Errorf("%s: v2 header missing width", path)
				}
			}
		}()
	}
	if !reached {
		t.Skip("no sample casts reachable")
	}
}
