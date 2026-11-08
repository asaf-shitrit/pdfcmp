package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExitCodesThresholdEdges covers verdicts that once depended on the
// threshold alone: NaN is a usage error, and byte mode decides on bytes.
func TestExitCodesThresholdEdges(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.pdf")
	b := filepath.Join(dir, "b.pdf")
	for path, data := range map[string]string{a: "aaa", b: "bbb"} {
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"NaN threshold", []string{"compare", "-q", "-t", "NaN", a, a}, exitError},
		{"byte mode different at threshold 0", []string{"compare", "-q", "-m", "byte", "-t", "0", a, b}, exitDifferent},
		{"byte mode same at threshold 0", []string{"compare", "-q", "-m", "byte", "-t", "0", a, a}, exitSame},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFlags(t, nil)
			if got := run(tt.args); got != tt.want {
				t.Errorf("pdfcmp %v: exit %d, want %d", tt.args, got, tt.want)
			}
		})
	}
}
