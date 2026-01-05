package compare

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestResultDifferentPages(t *testing.T) {
	tests := []struct {
		name     string
		results  []PageResult
		expected []int
	}{
		{
			name:     "NoPages",
			results:  nil,
			expected: nil,
		},
		{
			name: "AllSimilar",
			results: []PageResult{
				{Page: 1, Similarity: 0.98, IsDifferent: false},
				{Page: 2, Similarity: 0.99, IsDifferent: false},
			},
			expected: nil,
		},
		{
			name: "OneDifferent",
			results: []PageResult{
				{Page: 1, Similarity: 0.98, IsDifferent: false},
				{Page: 2, Similarity: 0.70, IsDifferent: true},
				{Page: 3, Similarity: 0.99, IsDifferent: false},
			},
			expected: []int{2},
		},
		{
			name: "MultipleDifferent",
			results: []PageResult{
				{Page: 1, Similarity: 0.50, IsDifferent: true},
				{Page: 2, Similarity: 0.60, IsDifferent: true},
				{Page: 3, Similarity: 0.99, IsDifferent: false},
				{Page: 4, Similarity: 0.40, IsDifferent: true},
			},
			expected: []int{1, 2, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Result{PageResults: tt.results}
			got := r.DifferentPages()

			if len(got) != len(tt.expected) {
				t.Errorf("DifferentPages() = %v, want %v", got, tt.expected)
				return
			}

			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("DifferentPages()[%d] = %d, want %d", i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestResultIsSimilar(t *testing.T) {
	tests := []struct {
		name      string
		score     float64
		threshold float64
		expected  bool
	}{
		{"ExactMatch", 1.0, 0.95, true},
		{"AboveThreshold", 0.96, 0.95, true},
		{"AtThreshold", 0.95, 0.95, true},
		{"BelowThreshold", 0.94, 0.95, false},
		{"ZeroScore", 0.0, 0.95, false},
		{"ZeroThreshold", 0.5, 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Result{SimilarityScore: tt.score}
			got := r.IsSimilar(tt.threshold)
			if got != tt.expected {
				t.Errorf("IsSimilar(%f) = %v, want %v", tt.threshold, got, tt.expected)
			}
		})
	}
}

func TestResultJSON(t *testing.T) {
	r := &Result{
		Identical:       true,
		SimilarityScore: 1.0,
		ByteComparison: ByteResult{
			Identical:  true,
			Hash1:      12345,
			Hash2:      12345,
			Size1:      1024,
			Size2:      1024,
			SizesMatch: true,
		},
		VisualComparison: VisualResult{
			OverallScore:     1.0,
			HashType:         "both",
			PagesCompared:    5,
			SamplingStrategy: "all",
			DPI:              150,
		},
		Metadata: Metadata{
			File1:      "/path/to/file1.pdf",
			File2:      "/path/to/file2.pdf",
			Pages1:     5,
			Pages2:     5,
			Size1:      1024,
			Size2:      1024,
			DurationMS: 100,
		},
		Duration:         100 * time.Millisecond,
		ComparisonLayers: []string{"bytewise", "visual"},
	}

	jsonStr, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("JSON output is not valid: %v", err)
	}

	// Verify key fields
	if parsed["identical"] != true {
		t.Error("JSON 'identical' field should be true")
	}
	if parsed["similarity_score"] != 1.0 {
		t.Error("JSON 'similarity_score' field should be 1.0")
	}
}

func TestResultSummary(t *testing.T) {
	tests := []struct {
		name           string
		result         Result
		expectedStatus string
		expectedParts  []string
	}{
		{
			name: "Identical",
			result: Result{
				Identical:       true,
				SimilarityScore: 1.0,
				Metadata: Metadata{
					File1:  "a.pdf",
					File2:  "b.pdf",
					Pages1: 1,
					Pages2: 1,
					Size1:  1024,
					Size2:  1024,
				},
				ByteComparison:   ByteResult{Identical: true},
				VisualComparison: VisualResult{OverallScore: 1.0, PagesCompared: 1},
				Duration:         10 * time.Millisecond,
			},
			expectedStatus: "IDENTICAL",
			expectedParts:  []string{"a.pdf", "b.pdf", "100.0%"},
		},
		{
			name: "Similar",
			result: Result{
				Identical:       false,
				SimilarityScore: 0.98,
				Metadata: Metadata{
					File1:  "a.pdf",
					File2:  "b.pdf",
					Pages1: 5,
					Pages2: 5,
					Size1:  2048,
					Size2:  2048,
				},
				ByteComparison:   ByteResult{Identical: false},
				VisualComparison: VisualResult{OverallScore: 0.98, PagesCompared: 5},
				Duration:         50 * time.Millisecond,
			},
			expectedStatus: "SIMILAR",
			expectedParts:  []string{"98.0%"},
		},
		{
			name: "Different",
			result: Result{
				Identical:       false,
				SimilarityScore: 0.50,
				Metadata: Metadata{
					File1:  "a.pdf",
					File2:  "b.pdf",
					Pages1: 10,
					Pages2: 10,
					Size1:  5000,
					Size2:  5000,
				},
				ByteComparison:   ByteResult{Identical: false},
				VisualComparison: VisualResult{OverallScore: 0.50, PagesCompared: 10},
				PageResults: []PageResult{
					{Page: 3, IsDifferent: true},
					{Page: 7, IsDifferent: true},
				},
				Duration: 100 * time.Millisecond,
			},
			expectedStatus: "DIFFERENT",
			expectedParts:  []string{"50.0%", "Modified pages"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := tt.result.Summary()

			if !strings.Contains(summary, tt.expectedStatus) {
				t.Errorf("Summary should contain status %q, got:\n%s", tt.expectedStatus, summary)
			}

			for _, part := range tt.expectedParts {
				if !strings.Contains(summary, part) {
					t.Errorf("Summary should contain %q, got:\n%s", part, summary)
				}
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.expected)
			}
		})
	}
}

func TestByteStatus(t *testing.T) {
	if byteStatus(true) != "IDENTICAL" {
		t.Error("byteStatus(true) should be IDENTICAL")
	}
	if byteStatus(false) != "DIFFERENT" {
		t.Error("byteStatus(false) should be DIFFERENT")
	}
}
