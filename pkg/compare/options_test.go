package compare

import (
	"runtime"
	"testing"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.Mode != ModeAuto {
		t.Errorf("expected ModeAuto, got %v", opts.Mode)
	}
	if opts.HashType != HashBoth {
		t.Errorf("expected HashBoth, got %v", opts.HashType)
	}
	if opts.DPI != 150 {
		t.Errorf("expected DPI 150, got %d", opts.DPI)
	}
	if opts.Workers != runtime.NumCPU() {
		t.Errorf("expected Workers %d, got %d", runtime.NumCPU(), opts.Workers)
	}
	if opts.SimilarityThreshold != 0.95 {
		t.Errorf("expected SimilarityThreshold 0.95, got %f", opts.SimilarityThreshold)
	}
	if opts.Sampling != SamplingAuto {
		t.Errorf("expected SamplingAuto, got %v", opts.Sampling)
	}
	if opts.MaxMemoryMB != 512 {
		t.Errorf("expected MaxMemoryMB 512, got %d", opts.MaxMemoryMB)
	}
}

func TestOptionsApply(t *testing.T) {
	opts := DefaultOptions()

	opts.Apply(
		WithMode(ModeFull),
		WithHashType(HashDHash),
		WithDPI(300),
		WithWorkers(4),
		WithThreshold(0.8),
		WithSampling(SamplingAll),
		WithMaxMemory(1024),
	)

	if opts.Mode != ModeFull {
		t.Errorf("expected ModeFull, got %v", opts.Mode)
	}
	if opts.HashType != HashDHash {
		t.Errorf("expected HashDHash, got %v", opts.HashType)
	}
	if opts.DPI != 300 {
		t.Errorf("expected DPI 300, got %d", opts.DPI)
	}
	if opts.Workers != 4 {
		t.Errorf("expected Workers 4, got %d", opts.Workers)
	}
	if opts.SimilarityThreshold != 0.8 {
		t.Errorf("expected SimilarityThreshold 0.8, got %f", opts.SimilarityThreshold)
	}
	if opts.Sampling != SamplingAll {
		t.Errorf("expected SamplingAll, got %v", opts.Sampling)
	}
	if opts.MaxMemoryMB != 1024 {
		t.Errorf("expected MaxMemoryMB 1024, got %d", opts.MaxMemoryMB)
	}
}

func TestOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{
			name:    "ValidDefaults",
			opts:    DefaultOptions(),
			wantErr: false,
		},
		{
			name: "ValidCustom",
			opts: Options{
				DPI:                 150,
				Workers:             4,
				SimilarityThreshold: 0.9,
				MaxMemoryMB:         256,
			},
			wantErr: false,
		},
		{
			name: "DPITooLow",
			opts: Options{
				DPI:                 10,
				Workers:             4,
				SimilarityThreshold: 0.9,
				MaxMemoryMB:         256,
			},
			wantErr: true,
		},
		{
			name: "DPITooHigh",
			opts: Options{
				DPI:                 2000,
				Workers:             4,
				SimilarityThreshold: 0.9,
				MaxMemoryMB:         256,
			},
			wantErr: true,
		},
		{
			name: "WorkersTooLow",
			opts: Options{
				DPI:                 150,
				Workers:             0,
				SimilarityThreshold: 0.9,
				MaxMemoryMB:         256,
			},
			wantErr: true,
		},
		{
			name: "WorkersTooHigh",
			opts: Options{
				DPI:                 150,
				Workers:             100,
				SimilarityThreshold: 0.9,
				MaxMemoryMB:         256,
			},
			wantErr: true,
		},
		{
			name: "ThresholdTooLow",
			opts: Options{
				DPI:                 150,
				Workers:             4,
				SimilarityThreshold: -0.1,
				MaxMemoryMB:         256,
			},
			wantErr: true,
		},
		{
			name: "ThresholdTooHigh",
			opts: Options{
				DPI:                 150,
				Workers:             4,
				SimilarityThreshold: 1.5,
				MaxMemoryMB:         256,
			},
			wantErr: true,
		},
		{
			name: "MaxMemoryNegative",
			opts: Options{
				DPI:                 150,
				Workers:             4,
				SimilarityThreshold: 0.9,
				MaxMemoryMB:         -1,
			},
			wantErr: true,
		},
		{
			name: "MaxMemoryZero",
			opts: Options{
				DPI:                 150,
				Workers:             4,
				SimilarityThreshold: 0.9,
				MaxMemoryMB:         0,
			},
			wantErr: true,
		},
		{
			name: "MultipleErrors",
			opts: Options{
				DPI:                 0,
				Workers:             0,
				SimilarityThreshold: 2.0,
				MaxMemoryMB:         -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWithProgress(t *testing.T) {
	var called bool
	callback := func(p Progress) {
		called = true
	}

	opts := DefaultOptions()
	opts.Apply(WithProgress(callback))

	if opts.ProgressCallback == nil {
		t.Error("ProgressCallback should not be nil")
	}

	// Invoke the callback
	opts.ProgressCallback(Progress{Stage: "test"})
	if !called {
		t.Error("ProgressCallback should have been called")
	}
}

func TestModeConstants(t *testing.T) {
	// Verify mode constants are distinct
	modes := []Mode{ModeAuto, ModeByte, ModeVisual, ModeFull}
	seen := make(map[Mode]bool)
	for _, m := range modes {
		if seen[m] {
			t.Errorf("Duplicate mode value: %v", m)
		}
		seen[m] = true
	}
}

func TestHashTypeConstants(t *testing.T) {
	// Verify hash type constants are distinct
	types := []HashType{HashBoth, HashDHash, HashPHash}
	seen := make(map[HashType]bool)
	for _, ht := range types {
		if seen[ht] {
			t.Errorf("Duplicate hash type value: %v", ht)
		}
		seen[ht] = true
	}
}

func TestSamplingStrategyConstants(t *testing.T) {
	// Verify sampling strategy constants are distinct
	strategies := []SamplingStrategy{SamplingAuto, SamplingAll, SamplingStrategic, SamplingThumbnailOnly}
	seen := make(map[SamplingStrategy]bool)
	for _, s := range strategies {
		if seen[s] {
			t.Errorf("Duplicate sampling strategy value: %v", s)
		}
		seen[s] = true
	}
}
