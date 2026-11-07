package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/asaf-shitrit/pdfcmp/pkg/compare"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	verbose bool
	quiet   bool
	format  string

	// Compare flags
	mode         string
	dpi          int
	workers      int
	threshold    float64
	hashType     string
	textHash     string
	textTolerant bool
	noTiles      bool
	sampling     string
	maxMemory    int
	timeout      time.Duration
)

// Exit codes, as cmp(1) and diff(1) use them.
const (
	exitSame      = 0 // the files are identical or similar
	exitDifferent = 1 // the files differ
	exitError     = 2 // the comparison could not be made
)

// version is set at build time with -ldflags "-X main.version=...".
var version string

// versionString returns the build's version: the one set at build time, else
// the module version go install recorded, else "dev".
func versionString() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

// exitCode is what main exits with once the command has run.
var exitCode = exitSame

func main() {
	os.Exit(run(os.Args[1:]))
}

// run executes the command line args and returns the process exit code.
func run(args []string) int {
	exitCode = exitSame
	rootCmd := newRootCmd()
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return exitError
	}
	return exitCode
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "pdfcmp",
		Short: "Lightning-fast PDF comparison engine with visual perception",
		Long: `pdfcmp is a high-performance comparison tool and library that uses a
multi-stage pipeline (byte-wise, text, and visual) to detect
identical or similar PDF documents with extreme speed.

Exit status is 0 if the files are identical or similar, 1 if they
differ, and 2 if they could not be compared.`,
		Version:       versionString(),
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if format != "text" && format != "json" {
				return fmt.Errorf("invalid format: %s (want text or json)", format)
			}
			// The arguments parsed; from here on an error is not a usage error.
			cmd.SilenceUsage = true
			return nil
		},
	}

	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress all output except result")
	rootCmd.PersistentFlags().StringVarP(&format, "format", "f", "text", "Output format: text, json")

	// Compare command
	compareCmd := &cobra.Command{
		Use:   "compare <file1.pdf> <file2.pdf>",
		Short: "Compare two PDF files",
		Long: `Compare two PDF files using a multi-layer pipeline:

  Layer 1: Byte-wise comparison (xxHash64)
  Layer 2: Thumbnail comparison (first/last pages at 72 DPI)
  Layer 3: Strategic sampling (sqrt(n)+2 pages)
  Layer 4: Full comparison (all pages if needed)`,
		Args: cobra.ExactArgs(2),
		RunE: runCompare,
	}

	compareCmd.Flags().StringVarP(&mode, "mode", "m", "auto", "Comparison mode: auto, byte, visual, full")
	compareCmd.Flags().IntVar(&dpi, "dpi", 150, "Rendering DPI for visual comparison")
	compareCmd.Flags().IntVarP(&workers, "workers", "w", runtime.NumCPU(), "Number of worker goroutines")
	compareCmd.Flags().Float64VarP(&threshold, "threshold", "t", 0.95, "Similarity threshold (0.0 to 1.0)")
	compareCmd.Flags().StringVar(&hashType, "hash", "both", "Hash type: dhash, phash, both")
	compareCmd.Flags().StringVar(&textHash, "text-hash", "minhash", "Text similarity for pages whose text differs: minhash, off")
	compareCmd.Flags().BoolVar(&noTiles, "no-tiles", false, "Score pages by their perceptual hashes alone, without the worst-tile check for small local changes")
	compareCmd.Flags().BoolVar(&textTolerant, "text-tolerant", false, "Let the threshold decide when a page's text changed, instead of calling the files different")
	compareCmd.Flags().StringVar(&sampling, "sampling", "auto", "Sampling strategy: auto, all, strategic, thumbnail")
	compareCmd.Flags().IntVar(&maxMemory, "memory", 512, "Memory budget in MB for page renders; fewer pages render at once to stay within it")
	compareCmd.Flags().DurationVar(&timeout, "timeout", 0, "Give up after this long, e.g. 30s or 5m (0 waits for the comparison to finish)")

	// Quick command
	quickCmd := &cobra.Command{
		Use:   "quick <file1.pdf> <file2.pdf>",
		Short: "Quick byte-wise comparison only",
		Long:  `Perform only byte-wise comparison using xxHash64. This is the fastest mode.`,
		Args:  cobra.ExactArgs(2),
		RunE:  runQuick,
	}

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "pdfcmp", versionString())
		},
	}

	rootCmd.AddCommand(compareCmd, quickCmd, versionCmd)
	return rootCmd
}

// Flag values accepted by the compare command.
var (
	modes = map[string]compare.Mode{
		"auto":   compare.ModeAuto,
		"byte":   compare.ModeByte,
		"visual": compare.ModeVisual,
		"full":   compare.ModeFull,
	}
	hashTypes = map[string]compare.HashType{
		"dhash": compare.HashDHash,
		"phash": compare.HashPHash,
		"both":  compare.HashBoth,
	}
	textHashes = map[string]compare.TextHashType{
		"minhash": compare.TextMinHash,
		"off":     compare.TextHashOff,
	}
	samplings = map[string]compare.SamplingStrategy{
		"auto":      compare.SamplingAuto,
		"all":       compare.SamplingAll,
		"strategic": compare.SamplingStrategic,
		"thumbnail": compare.SamplingThumbnailOnly,
	}
)

// lookup returns the value a flag's name stands for in table, or an error
// naming what kind of value was invalid.
func lookup[T any](table map[string]T, name, kind string) (T, error) {
	v, ok := table[name]
	if !ok {
		return v, fmt.Errorf("invalid %s: %s", kind, name)
	}
	return v, nil
}

// compareOptions turns the compare command's flags into comparer options.
func compareOptions() ([]compare.Option, error) {
	m, err := lookup(modes, mode, "mode")
	if err != nil {
		return nil, err
	}
	h, err := lookup(hashTypes, hashType, "hash type")
	if err != nil {
		return nil, err
	}
	th, err := lookup(textHashes, textHash, "text hash")
	if err != nil {
		return nil, err
	}
	smp, err := lookup(samplings, sampling, "sampling strategy")
	if err != nil {
		return nil, err
	}
	textChange := compare.TextChangeFails
	if textTolerant {
		textChange = compare.TextChangeScored
	}
	opts := []compare.Option{
		compare.WithTextChange(textChange),
		compare.WithTiles(!noTiles),
		compare.WithDPI(dpi),
		compare.WithWorkers(workers),
		compare.WithThreshold(threshold),
		compare.WithMaxMemory(maxMemory),
		compare.WithMode(m),
		compare.WithHashType(h),
		compare.WithTextHash(th),
		compare.WithSampling(smp),
	}
	if showProgress() {
		opts = append(opts, compare.WithProgress(func(p compare.Progress) {
			fmt.Fprintf(os.Stderr, "\r%s: %d/%d (%.0f%%)", p.Stage, p.CurrentPage, p.TotalPages, p.Percent*100)
		}))
	}
	return opts, nil
}

func showProgress() bool { return verbose && !quiet }

func runCompare(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// After the first signal, restore the default handling, so a second
	// one kills a comparison that is slow to stop.
	context.AfterFunc(ctx, stop)
	if timeout < 0 {
		return fmt.Errorf("invalid timeout: %v", timeout)
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	result, err := compareFiles(ctx, args[0], args[1])
	if err != nil {
		return err
	}
	if err := writeResult(cmd.OutOrStdout(), result); err != nil {
		return err
	}
	if !result.IsSimilar(threshold) {
		exitCode = exitDifferent
	}
	return nil
}

// compareFiles compares two PDFs with the options set by the command's flags.
func compareFiles(ctx context.Context, pdf1, pdf2 string) (*compare.Result, error) {
	if err := validateFiles(pdf1, pdf2); err != nil {
		return nil, err
	}
	opts, err := compareOptions()
	if err != nil {
		return nil, err
	}
	cmp, err := compare.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create comparer: %w", err)
	}
	defer func() { _ = cmp.Close() }()

	if !quiet {
		fmt.Fprintf(os.Stderr, "Comparing %s and %s...\n", pdf1, pdf2)
	}
	result, err := cmp.Compare(ctx, pdf1, pdf2)
	if showProgress() {
		fmt.Fprintln(os.Stderr) // end the progress line
	}
	if err != nil {
		return nil, fmt.Errorf("comparison failed: %w", err)
	}
	return result, nil
}

// writeResult writes a comparison result as JSON or as a text summary,
// depending on the --format flag.
func writeResult(w io.Writer, result *compare.Result) error {
	if format == "json" {
		jsonStr, err := result.JSON()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, jsonStr)
		return err
	}
	_, err := fmt.Fprint(w, result.Summary())
	return err
}

func runQuick(cmd *cobra.Command, args []string) error {
	identical, err := quickCompare(cmd.OutOrStdout(), args[0], args[1])
	if err != nil {
		return err
	}
	if !identical {
		exitCode = exitDifferent
	}
	return nil
}

// quickCompare compares two files byte for byte, writes whether they are
// identical, as JSON or as text depending on the --format flag, and returns
// it.
func quickCompare(w io.Writer, pdf1, pdf2 string) (bool, error) {
	if err := validateFiles(pdf1, pdf2); err != nil {
		return false, err
	}
	identical, err := compare.Quick(pdf1, pdf2)
	if err != nil {
		return false, fmt.Errorf("comparison failed: %w", err)
	}

	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return identical, enc.Encode(map[string]any{
			"identical": identical,
			"file1":     pdf1,
			"file2":     pdf2,
		})
	}
	verdict := "DIFFERENT: Files differ"
	if identical {
		verdict = "IDENTICAL: Files are byte-for-byte identical"
	}
	_, err = fmt.Fprintln(w, verdict)
	return identical, err
}

// validateFiles validates each path with validateFile.
func validateFiles(paths ...string) error {
	for _, p := range paths {
		if err := validateFile(p); err != nil {
			return err
		}
	}
	return nil
}

// validateFile checks if a file exists and is accessible, providing specific error messages
func validateFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", path)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied: cannot read %s", path)
		}
		return fmt.Errorf("cannot access file %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("expected a file but got a directory: %s", path)
	}
	return nil
}
