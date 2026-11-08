package compare

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWithLoggerNilIsNop(t *testing.T) {
	c, err := New(WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	path := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Must not panic on the nil logger.
	if _, err := c.Compare(context.Background(), path, path); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsNilLogger(t *testing.T) {
	o := DefaultOptions()
	o.Logger = nil
	if err := o.Validate(); err == nil {
		t.Error("Validate accepted a nil Logger")
	}
}

// Two results that differ only in unexported settings have different
// verdicts, and the JSON shows it.
func TestJSONCarriesVerdict(t *testing.T) {
	for _, tt := range []struct {
		mode    TextChangeMode
		status  string
		similar bool
	}{{TextChangeFails, "DIFFERENT", false}, {TextChangeScored, "SIMILAR", true}} {
		r := &Result{SimilarityScore: 0.99, TextChanged: true, textMode: tt.mode, threshold: 0.95, hasThreshold: true}
		s, err := r.JSON()
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(s), &got); err != nil {
			t.Fatal(err)
		}
		if got["status"] != tt.status || got["similar"] != tt.similar || got["threshold"] != 0.95 {
			t.Errorf("mode %d: status %v similar %v threshold %v", tt.mode, got["status"], got["similar"], got["threshold"])
		}
		if got["similarity_score"] != 0.99 {
			t.Errorf("existing field lost: %v", got["similarity_score"])
		}
		if r.IsSimilar(0.95) != tt.similar {
			t.Errorf("IsSimilar disagrees with similar")
		}
	}
}
