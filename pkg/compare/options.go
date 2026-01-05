package compare

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/asafshitrit/pdfcmp/pkg/render"
)

// Validation constants
const (
	MinDPI     = 36   // Minimum reasonable DPI
	MaxDPI     = 1200 // Maximum reasonable DPI (prevents memory issues)
	MinWorkers = 1
	MaxWorkers = 64
)

// Default values
const (
	DefaultSimilarityThreshold = 0.95 // Default threshold for similarity matching
	DefaultMaxMemoryMB         = 512  // Default memory limit in MB
	ThumbnailEarlyExitThresh   = 0.5  // Threshold below which thumbnail check exits early
)

// Mode specifies the comparison mode
type Mode int

const (
	// ModeAuto automatically chooses the best comparison strategy
	ModeAuto Mode = iota
	// ModeByte performs only byte-wise comparison
	ModeByte
	// ModeVisual performs only visual comparison
	ModeVisual
	// ModeFull performs both byte-wise and visual comparison
	ModeFull
)

// HashType specifies which perceptual hash algorithms to use
type HashType int

const (
	// HashBoth uses both dHash and pHash
	HashBoth HashType = iota
	// HashDHash uses only dHash (faster)
	HashDHash
	// HashPHash uses only pHash (more robust)
	HashPHash
)

// SamplingStrategy specifies how pages are sampled for comparison
type SamplingStrategy int

const (
	// SamplingAuto automatically chooses based on page count
	SamplingAuto SamplingStrategy = iota
	// SamplingAll compares all pages
	SamplingAll
	// SamplingStrategic uses strategic sampling (first, last, middle, sqrt distribution)
	SamplingStrategic
	// SamplingThumbnailOnly compares only first and last page thumbnails
	SamplingThumbnailOnly
)

// Options configures the comparison behavior
type Options struct {
	// Mode specifies what type of comparison to perform
	Mode Mode

	// HashType specifies which hash algorithms to use for visual comparison
	HashType HashType

	// DPI for rendering pages (default: 150)
	DPI int

	// Workers for concurrent page processing (default: NumCPU)
	Workers int

	// Threshold for considering pages "similar" (0.0 to 1.0, default: 0.95)
	SimilarityThreshold float64

	// Sampling strategy for page selection
	Sampling SamplingStrategy

	// MaxMemoryMB limits memory usage for large PDFs (default: 512)
	MaxMemoryMB int

	// ProgressCallback is called with progress updates
	ProgressCallback func(Progress)

	// Logger for structured logging (default: NopLogger)
	Logger Logger
}

// Progress represents comparison progress
type Progress struct {
	Stage       string  // Current stage name
	CurrentPage int     // Current page being processed
	TotalPages  int     // Total pages to process
	Percent     float64 // Overall progress percentage
}

// DefaultOptions returns sensible default options
func DefaultOptions() Options {
	return Options{
		Mode:                ModeAuto,
		HashType:            HashBoth,
		DPI:                 render.DPIStandard,
		Workers:             runtime.NumCPU(),
		SimilarityThreshold: DefaultSimilarityThreshold,
		Sampling:            SamplingAuto,
		MaxMemoryMB:         DefaultMaxMemoryMB,
		Logger:              NopLogger{},
	}
}

// Option is a functional option for configuring comparison
type Option func(*Options)

// WithMode sets the comparison mode
func WithMode(mode Mode) Option {
	return func(o *Options) {
		o.Mode = mode
	}
}

// WithHashType sets the hash algorithm(s) to use
func WithHashType(ht HashType) Option {
	return func(o *Options) {
		o.HashType = ht
	}
}

// WithDPI sets the rendering DPI
func WithDPI(dpi int) Option {
	return func(o *Options) {
		o.DPI = dpi
	}
}

// WithWorkers sets the number of worker goroutines
func WithWorkers(n int) Option {
	return func(o *Options) {
		o.Workers = n
	}
}

// WithThreshold sets the similarity threshold
func WithThreshold(t float64) Option {
	return func(o *Options) {
		o.SimilarityThreshold = t
	}
}

// WithSampling sets the sampling strategy
func WithSampling(s SamplingStrategy) Option {
	return func(o *Options) {
		o.Sampling = s
	}
}

// WithMaxMemory sets the maximum memory usage in MB
func WithMaxMemory(mb int) Option {
	return func(o *Options) {
		o.MaxMemoryMB = mb
	}
}

// WithProgress sets a progress callback
func WithProgress(fn func(Progress)) Option {
	return func(o *Options) {
		o.ProgressCallback = fn
	}
}

// WithLogger sets a structured logger for debugging and monitoring
func WithLogger(logger Logger) Option {
	return func(o *Options) {
		o.Logger = logger
	}
}

// Apply applies all options to a base Options struct
func (o *Options) Apply(opts ...Option) {
	for _, opt := range opts {
		opt(o)
	}
}

// Validate checks that all options are within acceptable ranges.
// Returns an error if any option is invalid.
func (o *Options) Validate() error {
	var errs []error

	if o.DPI < MinDPI || o.DPI > MaxDPI {
		errs = append(errs, fmt.Errorf("DPI must be between %d and %d, got %d", MinDPI, MaxDPI, o.DPI))
	}

	if o.Workers < MinWorkers || o.Workers > MaxWorkers {
		errs = append(errs, fmt.Errorf("workers must be between %d and %d, got %d", MinWorkers, MaxWorkers, o.Workers))
	}

	if o.SimilarityThreshold < 0.0 || o.SimilarityThreshold > 1.0 {
		errs = append(errs, fmt.Errorf("similarity threshold must be between 0.0 and 1.0, got %f", o.SimilarityThreshold))
	}

	if o.MaxMemoryMB <= 0 {
		errs = append(errs, fmt.Errorf("max memory must be positive, got %d", o.MaxMemoryMB))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
