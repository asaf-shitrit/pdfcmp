package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

// runJSON runs the command line as run does, with the command's output sent
// to a buffer, and returns the exit code and the JSON written. It leaves
// os.Stdout alone: PDFium instances bind to it when they are created.
func runJSON(t *testing.T, args []string) (int, map[string]any) {
	t.Helper()
	exitCode = exitSame
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pdfcmp %v: %v", args, err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("pdfcmp %v: output is not JSON: %v\n%s", args, err, out.String())
	}
	return exitCode, got
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
