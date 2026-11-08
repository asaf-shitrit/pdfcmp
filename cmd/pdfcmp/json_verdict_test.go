package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// runJSON runs the command line and returns its exit code and the JSON it
// wrote to standard output.
func runJSON(t *testing.T, args []string) (int, map[string]any) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := run(args)
	os.Stdout = old
	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("pdfcmp %v: output is not JSON: %v\n%s", args, err, out)
	}
	return code, got
}

// The JSON carries the verdict the exit code follows, for every flag that
// changes it.
func TestJSONVerdictAgreesWithExitCode(t *testing.T) {
	dir := t.TempDir()
	a := invoice(t, filepath.Join(dir, "a.pdf"), "1200.00")
	b := invoice(t, filepath.Join(dir, "b.pdf"), "1300.00")
	tests := []struct {
		name       string
		args       []string
		wantStatus string
		wantMode   string
		wantTiles  bool
	}{
		{"same file", []string{a, a}, "IDENTICAL", "fails", true},
		{"text change", []string{a, b}, "DIFFERENT", "fails", true},
		{"text change tolerated", []string{"--text-tolerant", a, b}, "SIMILAR", "scored", true},
		{"tolerated, threshold 1", []string{"--text-tolerant", "-t", "1", a, b}, "DIFFERENT", "scored", true},
		{"no tiles", []string{"--text-tolerant", "--no-tiles", a, b}, "SIMILAR", "scored", false},
		{"byte mode", []string{"-m", "byte", a, b}, "DIFFERENT", "fails", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFlags(t, nil)
			args := append([]string{"compare", "-q", "--format", "json"}, tt.args...)
			code, got := runJSON(t, args)
			if got["status"] != tt.wantStatus {
				t.Errorf("status = %v, want %s", got["status"], tt.wantStatus)
			}
			if got["text_change_mode"] != tt.wantMode || got["tiles"] != tt.wantTiles {
				t.Errorf("text_change_mode %v, tiles %v, want %s, %v", got["text_change_mode"], got["tiles"], tt.wantMode, tt.wantTiles)
			}
			similar, _ := got["similar"].(bool)
			wantCode := exitDifferent
			if similar {
				wantCode = exitSame
			}
			if code != wantCode {
				t.Errorf("similar = %v but exit code = %d", similar, code)
			}
			if _, ok := got["threshold"].(float64); !ok {
				t.Errorf("threshold missing: %v", got["threshold"])
			}
		})
	}
}
